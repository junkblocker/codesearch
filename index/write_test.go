// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"bytes"
	"os"
	"sort"
	"strings"
	"testing"
)

var trivialFiles = map[string]string{
	"f0":       "\n\n",
	"file1":    "\na\n",
	"thefile2": "\nab\n",
	"file3":    "\nabc\n",
	"afile4":   "\ndabc\n",
	"file5":    "\nxyzw\n",
}

var trivialIndex = join(
	// header
	"csearch index 1\n",

	// list of paths
	"\x00",

	// list of names
	"afile4\x00",
	"f0\x00",
	"file1\x00",
	"file3\x00",
	"file5\x00",
	"thefile2\x00",
	"\x00",

	// list of posting lists
	"\na\n", fileList(2), // file1
	"\nab", fileList(3, 5), // file3, thefile2
	"\nda", fileList(0), // afile4
	"\nxy", fileList(4), // file5
	"ab\n", fileList(5), // thefile2
	"abc", fileList(0, 3), // afile4, file3
	"bc\n", fileList(0, 3), // afile4, file3
	"dab", fileList(0), // afile4
	"xyz", fileList(4), // file5
	"yzw", fileList(4), // file5
	"zw\n", fileList(4), // file5
	"\xff\xff\xff", fileList(),

	// name index
	u32(0),
	u32(6+1),
	u32(6+1+2+1),
	u32(6+1+2+1+5+1),
	u32(6+1+2+1+5+1+5+1),
	u32(6+1+2+1+5+1+5+1+5+1),
	u32(6+1+2+1+5+1+5+1+5+1+8+1),

	// posting list index,
	"\na\n", u32(1), u32(0),
	"\nab", u32(2), u32(5),
	"\nda", u32(1), u32(5+6),
	"\nxy", u32(1), u32(5+6+5),
	"ab\n", u32(1), u32(5+6+5+5),
	"abc", u32(2), u32(5+6+5+5+5),
	"bc\n", u32(2), u32(5+6+5+5+5+6),
	"dab", u32(1), u32(5+6+5+5+5+6+6),
	"xyz", u32(1), u32(5+6+5+5+5+6+6+5),
	"yzw", u32(1), u32(5+6+5+5+5+6+6+5+5),
	"zw\n", u32(1), u32(5+6+5+5+5+6+6+5+5+5),
	"\xff\xff\xff", u32(0), u32(5+6+5+5+5+6+6+5+5+5+5),

	// trailer
	u32(16),
	u32(16+1),
	u32(16+1+38),
	u32(16+1+38+62),
	u32(16+1+38+62+28),

	"\ncsearch trailr\n",
)

func join(s ...string) string {
	return strings.Join(s, "")
}

func u32(x uint32) string {
	var buf [4]byte
	buf[0] = byte(x >> 24)
	buf[1] = byte(x >> 16)
	buf[2] = byte(x >> 8)
	buf[3] = byte(x)
	return string(buf[:])
}

func fileList(list ...int) string {
	var buf []byte

	last := -1
	for _, x := range list {
		delta := uint32(x - last)
		for delta >= 0x80 {
			buf = append(buf, byte(delta)|0x80)
			delta >>= 7
		}
		buf = append(buf, byte(delta))
		last = x
	}
	buf = append(buf, 0)
	return string(buf)
}

func buildFlushIndex(t *testing.T, out string, paths []string, doFlush bool, fileData map[string]string) {
	ix := Create(out)
	ix.AddPaths(paths)
	var files []string
	for name := range fileData {
		files = append(files, name)
	}
	sort.Strings(files)
	for _, name := range files {
		r := strings.NewReader(fileData[name])
		if err := ix.Add(name, r); err != nil {
			if t != nil {
				t.Logf("Add(%q): %v", name, err)
			}
		}
	}
	if doFlush {
		ix.flushPost()
	}
	ix.Flush()
}

func buildIndex(t *testing.T, name string, paths []string, fileData map[string]string) {
	buildFlushIndex(t, name, paths, false, fileData)
}

func testTrivialWrite(t *testing.T, doFlush bool) {
	// Force v1 index format so we can compare against the known trivialIndex byte string.
	oldVersion := writeVersion
	writeVersion = 1
	defer func() { writeVersion = oldVersion }()

	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()
	buildFlushIndex(t, out, nil, doFlush, trivialFiles)

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading index: %v", err)
	}
	want := []byte(trivialIndex)
	if !bytes.Equal(data, want) {
		i := 0
		for i < len(data) && i < len(want) && data[i] == want[i] {
			i++
		}
		t.Fatalf("wrong index:\nhave: %q %q\nwant: %q %q", data[:i], data[i:], want[:i], want[i:])
	}
}

func TestTrivialWrite(t *testing.T) {
	testTrivialWrite(t, false)
}

func TestTrivialWriteDisk(t *testing.T) {
	testTrivialWrite(t, true)
}

func TestHeap(t *testing.T) {
	h := &postHeap{}
	es := []postEntry{7, 4, 3, 2, 4}
	for _, e := range es {
		h.addMem([]postEntry{e})
	}
	if len(h.ch) != len(es) {
		t.Fatalf("wrong heap size: %d, want %d", len(h.ch), len(es))
	}
	for a, b := h.next(), h.next(); b.trigram() != (1<<24 - 1); a, b = b, h.next() {
		if a > b {
			t.Fatalf("%d should <= %d", a, b)
		}
	}
}

// indexedFiles returns the list of file names stored in the index at path.
func indexedFiles(t *testing.T, path string) []string {
	t.Helper()
	ix := Open(path)
	defer ix.Close()
	var names []string
	for i := 0; i < ix.numName; i++ {
		names = append(names, ix.Name(i).String())
	}
	return names
}

func TestMaxFileLen(t *testing.T) {
	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()

	ix := Create(out)
	ix.MaxFileLen = 5
	ix.Add("short", strings.NewReader("abc"))
	ix.Add("long", strings.NewReader("abcdefghij"))
	ix.Flush()

	names := indexedFiles(t, out)
	if len(names) != 1 || names[0] != "short" {
		t.Errorf("got indexed files %v, want [short]", names)
	}
}

func TestMaxLineLen(t *testing.T) {
	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()

	ix := Create(out)
	ix.MaxLineLen = 5
	// short lines — should be indexed
	ix.Add("ok", strings.NewReader("abc\ndef\n"))
	// line too long — should be skipped
	ix.Add("toolong", strings.NewReader("abcdefghij\n"))
	ix.Flush()

	names := indexedFiles(t, out)
	if len(names) != 1 || names[0] != "ok" {
		t.Errorf("got indexed files %v, want [ok]", names)
	}
}

func TestMaxTextTrigrams(t *testing.T) {
	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()

	// Build a string with many distinct trigrams.
	// Each unique 3-byte window is a trigram. Use varied bytes to maximise count.
	var bigContent strings.Builder
	for i := 0; i < 100; i++ {
		bigContent.WriteString("abcdefghijklmnopqrstuvwxyz")
	}
	big := bigContent.String()

	ix := Create(out)
	ix.MaxTextTrigrams = 5 // very low limit
	ix.Add("small", strings.NewReader("abc"))
	ix.Add("big", strings.NewReader(big))
	ix.Flush()

	names := indexedFiles(t, out)
	if len(names) != 1 || names[0] != "small" {
		t.Errorf("got indexed files %v, want [small]", names)
	}
}

func TestBinaryFileSkipped(t *testing.T) {
	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()

	ix := Create(out)
	ix.Add("text", strings.NewReader("hello world\n"))
	// NUL byte makes it binary
	ix.Add("binary", strings.NewReader("hel\x00lo\n"))
	ix.Flush()

	names := indexedFiles(t, out)
	if len(names) != 1 || names[0] != "text" {
		t.Errorf("got indexed files %v, want [text]", names)
	}
}

func TestMaxInvalidUTF8Ratio(t *testing.T) {
	f, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f.Name())
	out := f.Name()

	// Pure ASCII — should always be indexed.
	ascii := "hello world"
	// High ratio of invalid UTF-8 — should be skipped when ratio is 0.
	// \x80 is a bare continuation byte, invalid as a start byte.
	badUTF8 := "abc\x80\x81\x82\x83xyz"

	ix := Create(out)
	ix.MaxInvalidUTF8Ratio = 0.0 // zero tolerance
	ix.Add("ascii", strings.NewReader(ascii))
	ix.Add("badutf8", strings.NewReader(badUTF8))
	ix.Flush()

	names := indexedFiles(t, out)
	if len(names) != 1 || names[0] != "ascii" {
		t.Errorf("got indexed files %v, want [ascii]", names)
	}

	// With a generous ratio, the badutf8 file should be indexed too.
	f2, _ := os.CreateTemp("", "index-test")
	defer os.Remove(f2.Name())
	out2 := f2.Name()

	ix2 := Create(out2)
	ix2.MaxInvalidUTF8Ratio = 1.0 // tolerate all invalid bytes
	ix2.Add("ascii", strings.NewReader(ascii))
	ix2.Add("badutf8", strings.NewReader(badUTF8))
	ix2.Flush()

	names2 := indexedFiles(t, out2)
	if len(names2) != 2 {
		t.Errorf("got indexed files %v, want [ascii badutf8]", names2)
	}
}
