// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package index

import (
	"testing"
)

// makeTestIndex creates a minimal *Index just for driving deltaReader/Writer.
func makeTestIndexV2() *Index {
	return &Index{version: 2, name: "<test>"}
}

func makeTestIndexV1() *Index {
	return &Index{version: 1, name: "<test>"}
}

// deltaRoundTrip encodes values with a deltaWriter into a Buffer,
// then reads them back with a deltaReader.
func deltaRoundTrip(t *testing.T, ix *Index, values []int) []int {
	t.Helper()
	buf := bufCreate("")
	defer buf.file.Close()

	var w deltaWriter
	w.init(buf)
	for _, v := range values {
		w.Write(v)
	}
	w.Flush()
	buf.Flush()

	data, err := readBufBytes(buf)
	if err != nil {
		t.Fatalf("readBufBytes: %v", err)
	}

	var r deltaReader
	r.init(ix, data)
	var got []int
	for range values {
		got = append(got, r.next())
	}
	return got
}

func TestDeltaRoundTripV2(t *testing.T) {
	oldVersion := writeVersion
	writeVersion = 2
	defer func() { writeVersion = oldVersion }()

	ix := makeTestIndexV2()

	cases := []struct {
		name   string
		values []int
	}{
		{"single one", []int{1}},
		{"small sequence", []int{1, 2, 3, 4, 5}},
		{"large deltas", []int{1, 1000, 100000, 1 << 20}},
		// zero is a sentinel — it is encoded as deltaZeroEnc and decoded back to 0
		{"zero encoding", []int{0}},
		// Values spanning the deltaZeroEnc boundary (15, 16, 17, 18)
		{"boundary values", []int{15, 16, 17, 18, 31}},
		{"mixed", []int{1, 0, 5, 16, 0, 100}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deltaRoundTrip(t, ix, tc.values)
			if !equalList(got, tc.values) {
				t.Errorf("round-trip(%v) = %v", tc.values, got)
			}
		})
	}
}

func TestDeltaRoundTripV1(t *testing.T) {
	oldVersion := writeVersion
	writeVersion = 1
	defer func() { writeVersion = oldVersion }()

	ix := makeTestIndexV1()

	cases := []struct {
		name   string
		values []int
	}{
		{"single one", []int{1}},
		{"small sequence", []int{1, 2, 3, 4, 5}},
		{"large deltas", []int{1, 1000, 100000}},
		{"zero", []int{0}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deltaRoundTrip(t, ix, tc.values)
			if !equalList(got, tc.values) {
				t.Errorf("round-trip(%v) = %v", tc.values, got)
			}
		})
	}
}

// TestDeltaZeroEncodingSymmetry verifies that the sentinel value deltaZeroEnc
// round-trips correctly through the gamma encoder for both 0 (sentinel) and the
// real value deltaZeroEnc (which must be shifted).
func TestDeltaZeroEncodingSymmetry(t *testing.T) {
	oldVersion := writeVersion
	writeVersion = 2
	defer func() { writeVersion = oldVersion }()

	ix := makeTestIndexV2()

	// Write 0, deltaZeroEnc, deltaZeroEnc+1 and read them back.
	sequence := []int{0, deltaZeroEnc, deltaZeroEnc + 1}
	got := deltaRoundTrip(t, ix, sequence)
	if !equalList(got, sequence) {
		t.Errorf("zero-enc symmetry: got %v, want %v", got, sequence)
	}
}

// TestDeltaLargeValue ensures values requiring many bits encode/decode correctly.
func TestDeltaLargeValue(t *testing.T) {
	oldVersion := writeVersion
	writeVersion = 2
	defer func() { writeVersion = oldVersion }()

	ix := makeTestIndexV2()
	// 2^30 — requires 30-bit gamma code, crosses the 32-bit boundary in writeBits.
	large := []int{1 << 30}
	got := deltaRoundTrip(t, ix, large)
	if !equalList(got, large) {
		t.Errorf("large value: got %v, want %v", got, large)
	}
}
