// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2025 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"unsafe"

	"github.com/junkblocker/codesearch/sparse"
)

// Index writing.  See read.go for details of on-disk format.
//
// It would suffice to make a single large list of (trigram, file#) pairs
// while processing the files one at a time, sort that list by trigram,
// and then create the posting lists from subsequences of the list.
// However, we do not assume that the entire index fits in memory.
// Instead, we sort and flush the list to a new temporary file each time
// it reaches its maximum in-memory size, and then at the end we
// create the final posting lists by merging the temporary files as we
// read them back in.
//
// It would also be useful to be able to create an index for a subset
// of the files and then merge that index into an existing one.  This would
// allow incremental updating of an existing index when a directory changes.
// But we have not implemented that.

// An IndexWriter creates an on-disk index corresponding to a set of files.
type IndexWriter struct {
	LogSkip bool // log information about skipped files
	Verbose bool // log status using package log

	trigram *sparse.Set // trigrams for the current file
	buf     [8]byte     // scratch buffer

	paths []string

	nameData   *bufWriter // temp file holding list of names
	nameIndex  *bufWriter // temp file holding name index
	numName    int        // number of names written
	totalBytes int64

	post      []postEntry // list of (trigram, file#) pairs
	postFile  []*os.File  // flushed post entries
	postIndex *bufWriter  // temp file holding posting list index

	inbuf []byte     // input buffer
	main  *bufWriter // main index file

	MaxFileLen      int64
	MaxLineLen      int
	MaxTextTrigrams int

	MaxInvalidUTF8Ratio float64
}

const npost = 64 << 20 / 8 // 64 MB worth of post entries

// Create returns a new IndexWriter that will write the index to file.
func Create(file string) *IndexWriter {
	return &IndexWriter{
		trigram:             sparse.NewSet(1 << 24),
		nameData:            bufCreate(""),
		nameIndex:           bufCreate(""),
		postIndex:           bufCreate(""),
		main:                bufCreate(file),
		post:                make([]postEntry, 0, npost),
		inbuf:               make([]byte, 16384),
		MaxFileLen:          1 << 30,
		MaxLineLen:          2000,
		MaxTextTrigrams:     20000,
		MaxInvalidUTF8Ratio: 0.0,
	}
}

func (ix *IndexWriter) Close() {
	ix.main.finish().Close()
}

// A postEntry is an in-memory (trigram, file#) pair.
type postEntry uint64

func (p postEntry) trigram() uint32 {
	return uint32(p >> 32)
}

func (p postEntry) fileid() uint32 {
	return uint32(p)
}

func makePostEntry(trigram, fileid uint32) postEntry {
	return postEntry(trigram)<<32 | postEntry(fileid)
}

// AddPaths adds the given paths to the index's list of paths.
func (ix *IndexWriter) AddPaths(paths []string) {
	ix.paths = append(ix.paths, paths...)
}

// AddFile adds the file with the given name (opened using os.Open)
// to the index.  It logs errors using package log.
func (ix *IndexWriter) AddFile(name string) {
	fi, err := os.Stat(name)
	if err != nil {
		log.Print(err)
		return
	}
	f, err := os.Open(name)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	ix.Add(name, f, fi.Size())
}

// Add adds the file f to the index under the given name.
// It logs errors using package log.
func (ix *IndexWriter) Add(name string, f io.Reader, size int64) {
	if size > ix.MaxFileLen {
		if ix.LogSkip {
			log.Printf("%s: too long, ignoring\n", name)
		}
		return
	}
	ix.trigram.Reset()
	var (
		c           = byte(0)
		i           = 0
		buf         = ix.inbuf[:0]
		tv          = uint32(0)
		n           = int64(0)
		linelen     = 0
		inv_cnt     = int64(0)
		b1          = byte(0)
		b2          = byte(0)
		max_invalid = int64(float64(size) * ix.MaxInvalidUTF8Ratio)
	)
	for {
		tv = (tv << 8) & (1<<24 - 1)
		if i >= len(buf) {
			n, err := f.Read(buf[:cap(buf)])
			if n == 0 {
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Printf("%s: %v\n", name, err)
					return
				}
				log.Printf("%s: 0-length read\n", name)
				return
			}
			buf = buf[:n]
			i = 0
		}
		c = buf[i]
		i++
		tv |= uint32(c)
		if n++; n >= 3 {
			b1 = byte((tv >> 8) & 0xFF)
			b2 = byte(tv & 0xFF)
			if !validUTF8(b1, b2) {
				if inv_cnt++; inv_cnt > max_invalid {
					if ix.LogSkip {
						log.Printf("%s: skipped. High invalid UTF-8 ratio. total: %d invalid: %d ratio: %f\n", name, size, inv_cnt, float64(inv_cnt)/float64(size))
					}
					return
				}
			} else {
				ix.trigram.Add(tv)
			}
		}
		if (b1 == 0x00 || b2 == 0x00) && n >= 3 {
			if ix.LogSkip {
				log.Printf("%s: skipped. Binary file. Bytes %02X%02X at offset %d\n", name, (tv>>8)&0xFF, tv&0xFF, n)
			}
			return
		}
		if linelen++; linelen > ix.MaxLineLen {
			if ix.LogSkip {
				log.Printf("%s: skipped. Very long lines (%d)\n", name, linelen)
			}
			return
		}
		if c == '\n' {
			linelen = 0
		}
	}
	if inv_cnt > 0 {
		if (float64(inv_cnt) / float64(size)) > ix.MaxInvalidUTF8Ratio {
			if ix.LogSkip {
				log.Printf("%s: skipped. High invalid UTF-8 ratio. total: %d invalid: %d ratio: %f\n", name, size, inv_cnt, float64(inv_cnt)/float64(size))
			}
			return
		}
	}
	if ix.trigram.Len() > ix.MaxTextTrigrams {
		if ix.LogSkip {
			log.Printf("%s: skipped. Too many trigrams (%d > %d)\n", name, ix.trigram.Len(), ix.MaxTextTrigrams)
		}
		return
	}
	ix.totalBytes += n

	if ix.Verbose {
		log.Printf("%d %d %s\n", n, ix.trigram.Len(), name)
	}

	fileid := ix.addName(name)
	for _, trigram := range ix.trigram.Dense() {
		if len(ix.post) >= cap(ix.post) {
			ix.flushPost()
		}
		ix.post = append(ix.post, makePostEntry(trigram, fileid))
	}
}

// Flush flushes the index entry to the target file.
func (ix *IndexWriter) Flush() {
	ix.addName("")

	var off [5]uint32
	ix.main.writeString(magic)
	off[0] = ix.main.offset()
	for _, p := range ix.paths {
		ix.main.writeString(p)
		ix.main.writeString("\x00")
	}
	ix.main.writeString("\x00")
	off[1] = ix.main.offset()
	copyFile(ix.main, ix.nameData)
	off[2] = ix.main.offset()
	ix.mergePost(ix.main)
	off[3] = ix.main.offset()
	copyFile(ix.main, ix.nameIndex)
	off[4] = ix.main.offset()
	copyFile(ix.main, ix.postIndex)
	for _, v := range off {
		ix.main.writeUint32(v)
	}
	ix.main.writeString(trailerMagic)

	ix.nameData.file.Close()
	os.Remove(ix.nameData.name)
	for _, f := range ix.postFile {
		f.Close()
		os.Remove(f.Name())
	}
	ix.nameIndex.file.Close()
	os.Remove(ix.nameIndex.name)
	ix.postIndex.file.Close()
	os.Remove(ix.postIndex.name)

	log.Printf("%d data bytes, %d index bytes", ix.totalBytes, ix.main.offset())

	ix.main.flush()
}

func copyFile(dst, src *bufWriter) {
	dst.flush()
	_, err := io.Copy(dst.file, src.finish())
	if err != nil {
		log.Fatalf("copying %s to %s: %v", src.name, dst.name, err)
	}
}

// addName adds the file with the given name to the index.
// It returns the assigned file ID number.
func (ix *IndexWriter) addName(name string) uint32 {
	if strings.Contains(name, "\x00") {
		log.Fatalf("%q: file has NUL byte in name", name)
	}

	ix.nameIndex.writeUint32(ix.nameData.offset())
	ix.nameData.writeString(name)
	ix.nameData.writeByte(0)
	id := ix.numName
	ix.numName++
	return uint32(id)
}

// flushPost writes ix.post to a new temporary file and
// clears the slice.
func (ix *IndexWriter) flushPost() {
	w, err := ioutil.TempFile("", "csearch-index")
	if err != nil {
		log.Fatal(err)
	}
	if ix.Verbose {
		log.Printf("flush %d entries to %s", len(ix.post), w.Name())
	}
	sortPost(ix.post)

	// Write the raw ix.post array to disk as is.
	// This process is the one reading it back in, so byte order is not a concern.
	data := (*[npost * 8]byte)(unsafe.Pointer(&ix.post[0]))[:len(ix.post)*8]
	if n, err := w.Write(data); err != nil || n < len(data) {
		if err != nil {
			log.Fatal(err)
		}
		log.Fatalf("short write writing %s", w.Name())
	}

	ix.post = ix.post[:0]
	_, err = w.Seek(0, 0)
	if err != nil {
		log.Fatal(err)
	}
	ix.postFile = append(ix.postFile, w)
}

// mergePost reads the flushed index entries and merges them
// into posting lists, writing the resulting lists to out.
func (ix *IndexWriter) mergePost(out *bufWriter) {
	var h postHeap

	log.Printf("merge %d files + mem", len(ix.postFile))
	for _, f := range ix.postFile {
		h.addFile(f)
	}
	sortPost(ix.post)
	h.addMem(ix.post)

	npost := 0
	e := h.next()
	offset0 := out.offset()
	for {
		npost++
		offset := out.offset() - offset0
		trigram := e.trigram()
		ix.buf[0] = byte(trigram >> 16)
		ix.buf[1] = byte(trigram >> 8)
		ix.buf[2] = byte(trigram)

		// posting list
		fileid := ^uint32(0)
		nfile := uint32(0)
		out.write(ix.buf[:3])
		for ; e.trigram() == trigram && trigram != 1<<24-1; e = h.next() {
			out.writeUvarint(e.fileid() - fileid)
			fileid = e.fileid()
			nfile++
		}
		out.writeUvarint(0)

		// index entry
		ix.postIndex.write(ix.buf[:3])
		ix.postIndex.writeUint32(nfile)
		ix.postIndex.writeUint32(offset)

		if trigram == 1<<24-1 {
			break
		}
	}
	for _, mappedData := range h.mappedData {
		unmmapFile(mappedData)
	}
}

// A postChunk represents a chunk of post entries flushed to disk or
// still in memory.
type postChunk struct {
	// next entry
	head postEntry
	// remaining entries after head
	tail []postEntry
}

// A postHeap is a heap (priority queue) of postChunks.
type postHeap struct {
	ch         []*postChunk
	mappedData []*mmapData
}

func (h *postHeap) addFile(f *os.File) {
	mappedData := mmapFile(f)
	data := mappedData.data
	m := (*[npost]postEntry)(unsafe.Pointer(&data[0]))[:len(data)/8]
	h.addMem(m)
	// Make sure we close the mmap memory once we're done with it
	h.mappedData = append(h.mappedData, (&mappedData))
}

func (h *postHeap) addMem(x []postEntry) {
	h.add(&postChunk{tail: x})
}

// add adds the chunk to the postHeap.
// All adds must be called before the first call to next.
func (h *postHeap) add(ch *postChunk) {
	if len(ch.tail) > 0 {
		ch.head = ch.tail[0]
		ch.tail = ch.tail[1:]
		h.push(ch)
	}
}

// next returns the next entry from the postHeap.
// It returns a postEntry with trigram == 1<<24 - 1 if h is empty.
func (h *postHeap) next() postEntry {
	if len(h.ch) == 0 {
		return makePostEntry(1<<24-1, 0)
	}
	ch := h.ch[0]
	e := ch.head
	m := ch.tail
	if len(m) == 0 {
		h.pop()
	} else {
		ch.head = m[0]
		ch.tail = m[1:]
		h.siftDown(0)
	}
	return e
}

func (h *postHeap) pop() *postChunk {
	ch := h.ch[0]
	n := len(h.ch) - 1
	h.ch[0] = h.ch[n]
	h.ch = h.ch[:n]
	if n > 1 {
		h.siftDown(0)
	}
	return ch
}

func (h *postHeap) push(ch *postChunk) {
	n := len(h.ch)
	h.ch = append(h.ch, ch)
	if len(h.ch) >= 2 {
		h.siftUp(n)
	}
}

func (h *postHeap) siftDown(i int) {
	ch := h.ch
	for {
		j1 := 2*i + 1
		if j1 >= len(ch) {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < len(ch) && ch[j1].head >= ch[j2].head {
			j = j2
		}
		if ch[i].head < ch[j].head {
			break
		}
		ch[i], ch[j] = ch[j], ch[i]
		i = j
	}
}

func (h *postHeap) siftUp(j int) {
	ch := h.ch
	for {
		i := (j - 1) / 2
		if i == j || ch[i].head < ch[j].head {
			break
		}
		ch[i], ch[j] = ch[j], ch[i]
		j = i
	}
}

// A bufWriter is a convenience wrapper: a closeable bufio.Writer.
type bufWriter struct {
	name string
	file *os.File
	buf  []byte
}

// bufCreate creates a new file with the given name and returns a
// corresponding bufWriter.  If name is empty, bufCreate uses a
// temporary file.
func bufCreate(name string) *bufWriter {
	var (
		f   *os.File
		err error
	)
	if name != "" {
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	} else {
		f, err = ioutil.TempFile("", "csearch")
	}
	if err != nil {
		log.Fatal(err)
	}
	return &bufWriter{
		name: f.Name(),
		buf:  make([]byte, 0, 256<<10),
		file: f,
	}
}

func (b *bufWriter) write(x []byte) {
	n := cap(b.buf) - len(b.buf)
	if len(x) > n {
		b.flush()
		if len(x) >= cap(b.buf) {
			if _, err := b.file.Write(x); err != nil {
				log.Fatalf("writing %s: %v", b.name, err)
			}
			return
		}
	}
	b.buf = append(b.buf, x...)
}

func (b *bufWriter) writeByte(x byte) {
	if len(b.buf) >= cap(b.buf) {
		b.flush()
	}
	b.buf = append(b.buf, x)
}

func (b *bufWriter) writeString(s string) {
	n := cap(b.buf) - len(b.buf)
	if len(s) > n {
		b.flush()
		if len(s) >= cap(b.buf) {
			if _, err := b.file.WriteString(s); err != nil {
				log.Fatalf("writing %s: %v", b.name, err)
			}
			return
		}
	}
	b.buf = append(b.buf, s...)
}

// offset returns the current write offset.
func (b *bufWriter) offset() uint32 {
	off, err := b.file.Seek(0, 1)
	if err != nil {
		log.Fatal(err)
	}
	off += int64(len(b.buf))
	if int64(uint32(off)) != off {
		log.Fatal("index is larger than 4GB")
	}
	return uint32(off)
}

func (b *bufWriter) flush() {
	if len(b.buf) == 0 {
		return
	}
	_, err := b.file.Write(b.buf)
	if err != nil {
		log.Fatalf("writing %s: %v", b.name, err)
	}
	b.buf = b.buf[:0]
}

// finish flushes the file to disk and returns an open file ready for reading.
func (b *bufWriter) finish() *os.File {
	b.flush()
	f := b.file
	f.Seek(0, 0)
	return f
}

func (b *bufWriter) writeTrigram(t uint32) {
	if cap(b.buf)-len(b.buf) < 3 {
		b.flush()
	}
	b.buf = append(b.buf, byte(t>>16), byte(t>>8), byte(t))
}

func (b *bufWriter) writeUint32(x uint32) {
	if cap(b.buf)-len(b.buf) < 4 {
		b.flush()
	}
	b.buf = append(b.buf, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
}

func (b *bufWriter) writeUvarint(x uint32) {
	if cap(b.buf)-len(b.buf) < 5 {
		b.flush()
	}
	switch {
	case x < 1<<7:
		b.buf = append(b.buf, byte(x))
	case x < 1<<14:
		b.buf = append(b.buf, byte(x|0x80), byte(x>>7))
	case x < 1<<21:
		b.buf = append(b.buf, byte(x|0x80), byte(x>>7|0x80), byte(x>>14))
	case x < 1<<28:
		b.buf = append(b.buf, byte(x|0x80), byte(x>>7|0x80), byte(x>>14|0x80), byte(x>>21))
	default:
		b.buf = append(b.buf, byte(x|0x80), byte(x>>7|0x80), byte(x>>14|0x80), byte(x>>21|0x80), byte(x>>28))
	}
}

// validUTF8 reports whether the byte pair can appear in a
// valid sequence of UTF-8-encoded code points.
func validUTF8(c1, c2 byte) bool {
	switch {
	case c1 < 0x80:
		// 1-byte, must be followed by 1-byte or first of multi-byte
		return c2 < 0x80 || 0xc0 <= c2 && c2 < 0xf8
	case c1 < 0xc0:
		// continuation byte, can be followed by nearly anything
		return c2 < 0xf8
	case c1 < 0xf8:
		// first of multi-byte, must be followed by continuation byte
		return 0x80 <= c2 && c2 < 0xc0
	}
	return false
}

// sortPost sorts the postentry list.
// The list is already sorted by fileid (bottom 32 bits)
// and the top 8 bits are always zero, so there are only
// 24 bits to sort.  Run two rounds of 12-bit radix sort.
const sortK = 12

var sortTmp []postEntry
var sortN [1 << sortK]int

func sortPost(post []postEntry) {
	if len(post) > len(sortTmp) {
		sortTmp = make([]postEntry, len(post))
	}
	tmp := sortTmp[:len(post)]

	const k = sortK
	for i := range sortN {
		sortN[i] = 0
	}
	for _, p := range post {
		r := uintptr(p>>32) & (1<<k - 1)
		sortN[r]++
	}
	tot := 0
	for i, count := range sortN {
		sortN[i] = tot
		tot += count
	}
	for _, p := range post {
		r := uintptr(p>>32) & (1<<k - 1)
		o := sortN[r]
		sortN[r]++
		tmp[o] = p
	}
	tmp, post = post, tmp

	for i := range sortN {
		sortN[i] = 0
	}
	for _, p := range post {
		r := uintptr(p>>(32+k)) & (1<<k - 1)
		sortN[r]++
	}
	tot = 0
	for i, count := range sortN {
		sortN[i] = tot
		tot += count
	}
	for _, p := range post {
		r := uintptr(p>>(32+k)) & (1<<k - 1)
		o := sortN[r]
		sortN[r]++
		tmp[o] = p
	}
}
