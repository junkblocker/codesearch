// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// V2 index full write / read round-trip
// ---------------------------------------------------------------------------

// TestV2IndexRoundTrip builds a v2 index with known content and verifies
// Name(), PostingList(), Roots(), and Check() all work correctly.
func TestV2IndexRoundTrip(t *testing.T) {
	f, _ := os.CreateTemp("", "index-v2-test")
	defer os.Remove(f.Name())
	out := f.Name()

	paths := []string{"/src"}
	files := map[string]string{
		"/src/alpha": "hello world",
		"/src/beta":  "goodbye world",
		"/src/gamma": "hello again",
	}
	buildIndex(t, out, paths, files)

	ix := Open(out)
	defer ix.Close()

	// Version must be 2 (default writeVersion).
	if ix.version != 2 {
		t.Fatalf("expected version 2, got %d", ix.version)
	}

	// Roots
	var roots []string
	for p := range ix.Roots().All() {
		roots = append(roots, p.String())
	}
	if !slices.Equal(roots, []string{"/src"}) {
		t.Errorf("Roots() = %v, want [/src]", roots)
	}

	// All three files should be indexed (sorted order).
	want := []string{"/src/alpha", "/src/beta", "/src/gamma"}
	var names []string
	for i := range ix.numName {
		names = append(names, ix.Name(i).String())
	}
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}

	// "wor" appears in alpha (0) and beta (1).
	l := ix.PostingList(tri('w', 'o', 'r'))
	if !equalList(l, []int{0, 1}) {
		t.Errorf("PostingList(wor) = %v, want [0 1]", l)
	}

	// "hel" appears in alpha (0) and gamma (2).
	l = ix.PostingList(tri('h', 'e', 'l'))
	if !equalList(l, []int{0, 2}) {
		t.Errorf("PostingList(hel) = %v, want [0 2]", l)
	}

	// Check must pass on a freshly-created v2 index.
	if err := ix.Check(); err != nil {
		t.Errorf("Check() = %v, want nil", err)
	}
}

// TestV2CheckOnV1Index verifies that Check() is a no-op for v1 indexes.
func TestV2CheckOnV1Index(t *testing.T) {
	oldVersion := writeVersion
	writeVersion = 1
	defer func() { writeVersion = oldVersion }()

	f, _ := os.CreateTemp("", "index-v1-check")
	defer os.Remove(f.Name())
	out := f.Name()

	buildIndex(t, out, nil, map[string]string{"afile": "hello"})

	ix := Open(out)
	defer ix.Close()

	if ix.version != 1 {
		t.Fatalf("expected v1 index, got version %d", ix.version)
	}
	if err := ix.Check(); err != nil {
		t.Errorf("Check() on v1 index = %v, want nil", err)
	}
}

// ---------------------------------------------------------------------------
// Re-merge: merge a v2 index with another v2 index (sentinel regression)
// ---------------------------------------------------------------------------

// TestReMergeV2 catches the "no progress" panic that was triggered when
// re-merging a previously merged v2 index (the invalidTrigram sentinel
// was being written into the posting index by mistake).
func TestReMergeV2(t *testing.T) {
	// Build ix1 and ix2, merge them into ix3, then merge ix3 with ix4 into ix5.
	// If the sentinel bug regresses, ix3 merge or the second merge panics.
	tmp := func() (string, func()) {
		f, err := os.CreateTemp("", "index-remerge")
		if err != nil {
			t.Fatal(err)
		}
		return f.Name(), func() { os.Remove(f.Name()) }
	}

	a, da := tmp()
	defer da()
	b, db := tmp()
	defer db()
	ab, dab := tmp()
	defer dab()
	c, dc := tmp()
	defer dc()
	abc, dabc := tmp()
	defer dabc()

	buildIndex(t, a, []string{"/a"}, map[string]string{
		"/a/x": "now is the time for all good men",
		"/a/y": "hello world",
	})
	buildIndex(t, b, []string{"/b"}, map[string]string{
		"/b/x": "now or never",
		"/b/y": "goodbye world",
	})
	Merge(ab, a, b)

	// Verify the first merge is correct.
	ixAB := Open(ab)
	if err := ixAB.Check(); err != nil {
		t.Fatalf("Check after first merge: %v", err)
	}
	ixAB.Close()

	buildIndex(t, c, []string{"/c"}, map[string]string{
		"/c/z": "all the time in the world",
	})

	// Second merge: merge(ab, c) — this triggered the panic before the fix.
	Merge(abc, ab, c)

	ixABC := Open(abc)
	defer ixABC.Close()

	if err := ixABC.Check(); err != nil {
		t.Fatalf("Check after second merge: %v", err)
	}

	// "now" should appear in /a/x (0) and /b/x (2).
	l := ixABC.PostingList(tri('n', 'o', 'w'))
	if len(l) < 2 {
		t.Errorf("PostingList(now) = %v, want at least 2 entries", l)
	}

	// "wor" should appear in /a/y (1), /b/y (3), and /c/z (4).
	l = ixABC.PostingList(tri('w', 'o', 'r'))
	if len(l) < 3 {
		t.Errorf("PostingList(wor) = %v, want at least 3 entries", l)
	}
}

// ---------------------------------------------------------------------------
// MergeOr
// ---------------------------------------------------------------------------

func TestMergeOrEmpty(t *testing.T) {
	if l := mergeOr(nil, nil); l != nil {
		t.Errorf("mergeOr(nil, nil) = %v, want nil", l)
	}
	if l := mergeOr([]int{1, 2}, nil); !equalList(l, []int{1, 2}) {
		t.Errorf("mergeOr([1,2], nil) = %v, want [1 2]", l)
	}
	if l := mergeOr(nil, []int{3, 4}); !equalList(l, []int{3, 4}) {
		t.Errorf("mergeOr(nil, [3,4]) = %v, want [3 4]", l)
	}
}

func TestMergeOrOverlapping(t *testing.T) {
	l1 := []int{1, 3, 5}
	l2 := []int{2, 3, 4}
	got := mergeOr(l1, l2)
	want := []int{1, 2, 3, 4, 5}
	if !equalList(got, want) {
		t.Errorf("mergeOr(%v, %v) = %v, want %v", l1, l2, got, want)
	}
}

func TestMergeOrDisjoint(t *testing.T) {
	l1 := []int{1, 2}
	l2 := []int{3, 4}
	got := mergeOr(l1, l2)
	want := []int{1, 2, 3, 4}
	if !equalList(got, want) {
		t.Errorf("mergeOr(%v, %v) = %v, want %v", l1, l2, got, want)
	}
}

func TestMergeOrDuplicates(t *testing.T) {
	l := []int{1, 2, 3}
	got := mergeOr(l, l)
	if !equalList(got, l) {
		t.Errorf("mergeOr(l, l) = %v, want %v", got, l)
	}
}

// ---------------------------------------------------------------------------
// postEntry encode/decode
// ---------------------------------------------------------------------------

func TestPostEntryRoundTrip(t *testing.T) {
	cases := []struct {
		trigram uint32
		fileid  int
	}{
		{0, 0},
		{1, 1},
		{tri('a', 'b', 'c'), 42},
		{tri('z', 'z', 'z'), 1<<24 - 2},
		{invalidTrigram - 1, 999999},
	}
	for _, tc := range cases {
		pe := makePostEntry(tc.trigram, tc.fileid)
		if got := pe.trigram(); got != tc.trigram {
			t.Errorf("makePostEntry(%d, %d).trigram() = %d, want %d",
				tc.trigram, tc.fileid, got, tc.trigram)
		}
		if got := pe.fileid(); got != tc.fileid {
			t.Errorf("makePostEntry(%d, %d).fileid() = %d, want %d",
				tc.trigram, tc.fileid, got, tc.fileid)
		}
	}
}

func TestPostEntryOrdering(t *testing.T) {
	// postEntry is ordered first by trigram, then by fileid (via the packed uint64).
	a := makePostEntry(1, 5)
	b := makePostEntry(1, 10)
	c := makePostEntry(2, 1)
	if !(a < b) {
		t.Errorf("expected %v < %v (same trigram, lower fileid first)", a, b)
	}
	if !(b < c) {
		t.Errorf("expected %v < %v (lower trigram first)", b, c)
	}
}

// ---------------------------------------------------------------------------
// isValidName
// ---------------------------------------------------------------------------

func TestIsValidName(t *testing.T) {
	valid := []string{
		"/foo/bar",
		"simple",
		"/a/b/c.go",
		" space",  // space (0x20) is allowed
		"!@#$%",
	}
	invalid := []string{
		"",
		"\x00null",
		"con\ttrol",
		"\x1fnon-printable",
	}
	for _, s := range valid {
		if !isValidName(s) {
			t.Errorf("isValidName(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if isValidName(s) {
			t.Errorf("isValidName(%q) = true, want false", s)
		}
	}
}

// ---------------------------------------------------------------------------
// validUTF8
// ---------------------------------------------------------------------------

func TestValidUTF8(t *testing.T) {
	cases := []struct {
		c1, c2 uint32
		want   bool
	}{
		// ASCII followed by ASCII
		{'a', 'b', true},
		// ASCII followed by start of multi-byte
		{'a', 0xC0, true},
		// ASCII followed by 0xF8+ (invalid)
		{'a', 0xF8, false},
		// Continuation byte followed by continuation byte
		{0x80, 0x80, true},
		// Continuation byte followed by start of high multi-byte
		{0x80, 0xE0, true},
		// Start of 2-byte followed by continuation byte
		{0xC2, 0x80, true},
		// Start of 2-byte followed by non-continuation
		{0xC2, 'a', false},
		// Start of 4-byte followed by continuation byte
		{0xF0, 0x90, true},
		// Start of 4-byte followed by non-continuation
		{0xF0, 'x', false},
	}
	for _, tc := range cases {
		got := validUTF8(tc.c1, tc.c2)
		if got != tc.want {
			t.Errorf("validUTF8(%#x, %#x) = %v, want %v", tc.c1, tc.c2, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// NamesAt / Names iterator correctness
// ---------------------------------------------------------------------------

func TestNamesAtV2(t *testing.T) {
	f, _ := os.CreateTemp("", "index-namesat")
	defer os.Remove(f.Name())

	// Build an index with many files so the name group boundary is crossed.
	files := make(map[string]string)
	var want []string
	for i := 0; i < 35; i++ {
		name := strings.Repeat("a", i+1)
		files[name] = "x"
		want = append(want, name)
	}
	buildIndex(t, f.Name(), nil, files)

	ix := Open(f.Name())
	defer ix.Close()

	// Verify every file can be retrieved individually.
	for i, w := range want {
		got := ix.Name(i).String()
		if got != w {
			t.Errorf("Name(%d) = %q, want %q", i, got, w)
		}
	}

	// Verify Names(lo, hi) for various ranges.
	for _, tc := range []struct{ lo, hi int }{{0, 1}, {0, 16}, {15, 17}, {16, 32}, {0, 35}, {10, 25}} {
		var got []string
		for p := range ix.Names(tc.lo, tc.hi) {
			got = append(got, p.String())
		}
		if len(got) != tc.hi-tc.lo {
			t.Errorf("Names(%d,%d): got %d names, want %d", tc.lo, tc.hi, len(got), tc.hi-tc.lo)
		}
		if !slices.Equal(got, want[tc.lo:tc.hi]) {
			t.Errorf("Names(%d,%d) = %v, want %v", tc.lo, tc.hi, got, want[tc.lo:tc.hi])
		}
	}
}

// ---------------------------------------------------------------------------
// PostingQuery
// ---------------------------------------------------------------------------

func TestPostingQueryAll(t *testing.T) {
	f, _ := os.CreateTemp("", "index-query")
	defer os.Remove(f.Name())

	buildIndex(t, f.Name(), nil, map[string]string{
		"file0": "hello",
		"file1": "world",
		"file2": "hello world",
	})

	ix := Open(f.Name())
	defer ix.Close()

	all := ix.PostingQuery(&Query{Op: QAll})
	if !equalList(all, []int{0, 1, 2}) {
		t.Errorf("QAll = %v, want [0 1 2]", all)
	}

	none := ix.PostingQuery(&Query{Op: QNone})
	if none != nil {
		t.Errorf("QNone = %v, want nil", none)
	}
}

func TestPostingQueryAndOr(t *testing.T) {
	f, _ := os.CreateTemp("", "index-query-and")
	defer os.Remove(f.Name())

	buildIndex(t, f.Name(), nil, map[string]string{
		"afile": "hello world",
		"bfile": "goodbye world",
		"cfile": "hello again",
	})

	ix := Open(f.Name())
	defer ix.Close()

	// "hel" AND "wor": only file 0 (afile has both)
	andQ := &Query{
		Op:      QAnd,
		Trigram: []string{"hel", "wor"},
	}
	and := ix.PostingQuery(andQ)
	if !equalList(and, []int{0}) {
		t.Errorf("QAnd(hel,wor) = %v, want [0]", and)
	}

	// "hel" OR "goo": file 0 and file 2 (hel), file 1 (goo)
	orQ := &Query{
		Op:      QOr,
		Trigram: []string{"hel", "goo"},
	}
	or := ix.PostingQuery(orQ)
	if len(or) < 2 {
		t.Errorf("QOr(hel,goo) = %v, want at least 2 files", or)
	}
}

// ---------------------------------------------------------------------------
// Merge preserves posting lists across root shadow boundaries
// ---------------------------------------------------------------------------

func TestMergeRootShadowing(t *testing.T) {
	f1, _ := os.CreateTemp("", "index-shadow1")
	f2, _ := os.CreateTemp("", "index-shadow2")
	f3, _ := os.CreateTemp("", "index-shadow3")
	defer os.Remove(f1.Name())
	defer os.Remove(f2.Name())
	defer os.Remove(f3.Name())

	// ix1: /a/old contains "potato"
	buildIndex(t, f1.Name(), []string{"/a"}, map[string]string{
		"/a/old": "potato",
		"/b/keep": "world",
	})
	// ix2: /a/new replaces everything under /a — ix2 claims /a.
	buildIndex(t, f2.Name(), []string{"/a"}, map[string]string{
		"/a/new": "potato soup",
	})

	Merge(f3.Name(), f1.Name(), f2.Name())

	ix := Open(f3.Name())
	defer ix.Close()

	if err := ix.Check(); err != nil {
		t.Fatalf("Check after root-shadow merge: %v", err)
	}

	// /a/old should be gone (shadowed by ix2's /a root).
	// /a/new and /b/keep should remain.
	var names []string
	for i := range ix.numName {
		names = append(names, ix.Name(i).String())
	}
	for _, n := range names {
		if n == "/a/old" {
			t.Errorf("expected /a/old to be shadowed out, but it appears in merged index: %v", names)
		}
	}
	found := slices.Contains(names, "/a/new") && slices.Contains(names, "/b/keep")
	if !found {
		t.Errorf("merged names %v should contain /a/new and /b/keep", names)
	}
}
