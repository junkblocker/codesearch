// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2023 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"log"
	"os"
	"syscall"
)

func mmapFile(f *os.File) mmapData {
	st, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	size := st.Size()
	if int64(int(size+4095)) != size+4095 {
		log.Fatalf("%s: too large for mmap", f.Name())
	}
	n := int(size)
	if n == 0 {
		return mmapData{f, nil, 0}
	}
	data, err := syscall.Mmap(int(f.Fd()), 0, (n+4095)&^4095, syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		log.Fatalf("mmap %s: %v", f.Name(), err)
	}
	return mmapData{f, data[:n], 0}
}

func unmmapFile(mm *mmapData) {
	len_equal_cap := mm.data[0:cap(mm.data)] // else get EINVAL
	err := syscall.Munmap(len_equal_cap)
	if err != nil {
		log.Println("unmmapFile:", err)
	}
}
