// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"slices"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Path.Compare
// ---------------------------------------------------------------------------

func TestPathCompare(t *testing.T) {
	// Slashes sort before any printable character (treated as \x00).
	// So "/a/b" < "/a.b" because '/' (0) < '.' (0x2e).
	cases := []struct {
		a, b string
		want int // sign only
	}{
		{"/a/b", "/a.b", -1},  // slash < dot
		{"/a.b", "/a/b", +1}, // dot > slash
		{"/a/b", "/a/b", 0},  // equal
		{"/a", "/a/b", -1},   // prefix is less
		{"/a/b", "/a", +1},   // longer is more
		{"", "", 0},
		{"a", "b", -1},
		{"b", "a", +1},
		// directory before file with same prefix but different byte
		{"/foo/bar", "/foobar", -1}, // slash treated as 0 < 'b'
	}
	for _, tc := range cases {
		p := MakePath(tc.a)
		q := MakePath(tc.b)
		got := p.Compare(q)
		if sign(got) != tc.want {
			t.Errorf("MakePath(%q).Compare(MakePath(%q)) = %d, want sign %+d",
				tc.a, tc.b, got, tc.want)
		}
	}
}

func sign(x int) int {
	switch {
	case x < 0:
		return -1
	case x > 0:
		return +1
	default:
		return 0
	}
}

func TestPathCompareTransitivity(t *testing.T) {
	paths := []string{
		"/a", "/a/b", "/a/b/c", "/a.b", "/a/b.c", "/b", "/foo/bar", "/foo.bar",
	}
	for i, a := range paths {
		for j, b := range paths {
			for _, c := range paths[j+1:] {
				pa := MakePath(a)
				pb := MakePath(b)
				pc := MakePath(c)
				ab := pa.Compare(pb)
				bc := pb.Compare(pc)
				ac := pa.Compare(pc)
				if ab < 0 && bc < 0 && ac >= 0 {
					t.Errorf("transitivity violation: %q < %q < %q but %q.cmp(%q)=%d (paths[%d])",
						a, b, c, a, c, ac, i)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Path.HasPathPrefix
// ---------------------------------------------------------------------------

func TestHasPathPrefix(t *testing.T) {
	cases := []struct {
		child, parent string
		want          bool
	}{
		{"/foo/bar", "/foo", true},
		{"/foo/bar", "/foo/bar", true},
		{"/foo/bar/baz", "/foo/bar", true},
		{"/foobar", "/foo", false},   // no separator
		{"/foo", "/foo/bar", false},  // parent longer than child
		{"/foo", "/foo", true},
		{"", "", false},              // empty parent
		{"/foo", "", false},          // empty parent
	}
	for _, tc := range cases {
		p := MakePath(tc.child)
		parent := MakePath(tc.parent)
		got := p.HasPathPrefix(parent)
		if got != tc.want {
			t.Errorf("MakePath(%q).HasPathPrefix(MakePath(%q)) = %v, want %v",
				tc.child, tc.parent, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PathWriter / PathReader round-trip (v2)
// ---------------------------------------------------------------------------

func TestPathWriterReaderV2RoundTrip(t *testing.T) {
	inputs := []string{
		"/a",
		"/a/b",
		"/a/b/c",
		"/a/b/d",
		"/a/c",
		"/b",
		"/b/x",
		"/foo/bar/baz",
		"/foo/bar/qux",
		"/zoo",
	}

	buf := bufCreate("")
	defer func() { buf.file.Close() }()

	pw := NewPathWriter(buf, nil, 2, 0)
	for _, s := range inputs {
		pw.Write(MakePath(s))
	}
	buf.Flush()

	data, err := readBufBytes(buf)
	if err != nil {
		t.Fatal(err)
	}

	pr := NewPathReader(2, data, len(inputs))
	var got []string
	for p := range pr.All() {
		got = append(got, p.String())
	}

	if !slices.Equal(got, inputs) {
		t.Errorf("round-trip mismatch:\ngot  %v\nwant %v", got, inputs)
	}
}

func TestPathWriterReaderV1RoundTrip(t *testing.T) {
	inputs := []string{"aaa", "bbb", "ccc", "ddddd"}

	buf := bufCreate("")
	defer func() { buf.file.Close() }()

	pw := NewPathWriter(buf, nil, 1, 0)
	for _, s := range inputs {
		pw.Write(MakePath(s))
	}
	// v1 requires trailing empty sentinel
	pw.Write(MakePath(""))
	buf.Flush()

	data, err := readBufBytes(buf)
	if err != nil {
		t.Fatal(err)
	}

	// v1 PathReader stops at empty path (limit -1 == unlimited)
	pr := NewPathReader(1, data, -1)
	var got []string
	for p := range pr.All() {
		got = append(got, p.String())
	}

	if !slices.Equal(got, inputs) {
		t.Errorf("round-trip mismatch:\ngot  %v\nwant %v", got, inputs)
	}
}

func TestPathWriterCount(t *testing.T) {
	buf := bufCreate("")
	defer func() { buf.file.Close() }()

	pw := NewPathWriter(buf, nil, 2, 0)
	for i, s := range []string{"/a", "/b", "/c"} {
		pw.Write(MakePath(s))
		if pw.Count() != i+1 {
			t.Errorf("Count after %d writes = %d, want %d", i+1, pw.Count(), i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// PathWriter / PathReader with grouping (nameGroupSize)
// ---------------------------------------------------------------------------

func TestPathWriterGroupedIndexRoundTrip(t *testing.T) {
	// Build more paths than one group so the index has multiple entries.
	var inputs []string
	for i := 0; i < 40; i++ {
		inputs = append(inputs, strings.Repeat("a", i+1))
	}

	data := bufCreate("")
	defer func() { data.file.Close() }()
	idx := bufCreate("")
	defer func() { idx.file.Close() }()

	pw := NewPathWriter(data, idx, 2, nameGroupSize)
	for _, s := range inputs {
		pw.Write(MakePath(s))
	}
	data.Flush()
	idx.Flush()

	rawData, err := readBufBytes(data)
	if err != nil {
		t.Fatal(err)
	}

	pr := NewPathReader(2, rawData, len(inputs))
	var got []string
	for p := range pr.All() {
		got = append(got, p.String())
	}

	if !slices.Equal(got, inputs) {
		t.Errorf("grouped round-trip mismatch:\ngot  %v\nwant %v", got, inputs)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// readBufBytes reads all written bytes from a Buffer (seeks to start).
func readBufBytes(b *Buffer) ([]byte, error) {
	f := b.finish()
	size, err := f.Seek(0, 2) // seek to end to get size
	if err != nil {
		return nil, err
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]byte, size)
	_, err = f.Read(out)
	return out, err
}
