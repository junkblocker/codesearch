package index

import (
	"testing"
)

func TestPathReaderNumPaths(t *testing.T) {
	paths := []string{"/a/b", "/a/c", "/a/d"}
	buf := bufCreate("")
	pw := NewPathWriter(buf, nil, 2, 0)
	for _, p := range paths {
		pw.Write(MakePath(p))
	}
	buf.Flush()

	data := make([]byte, buf.Offset())
	buf.file.Seek(0, 0)
	buf.file.Read(data)

	pr := NewPathReader(2, data, len(paths))
	for pr.Valid() {
		if !pr.Next() {
			break
		}
	}
	if pr.NumPaths() != len(paths) {
		t.Errorf("NumPaths() = %d, want %d", pr.NumPaths(), len(paths))
	}
}

func TestHasPathPrefixEmptyParent(t *testing.T) {
	p := MakePath("/foo/bar")
	empty := MakePath("")
	if p.HasPathPrefix(empty) {
		t.Error("HasPathPrefix with empty parent should return false")
	}
}
