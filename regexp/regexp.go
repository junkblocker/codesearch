// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2025 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package regexp implements regular expression search tuned for
// use in grep-like programs.
package regexp

import "regexp/syntax"

func bug() {
	panic("codesearch/regexp: internal error")
}

// Regexp is the representation of a compiled regular expression.
// A Regexp is NOT SAFE for concurrent use by multiple goroutines.
type Regexp struct {
	Syntax  *syntax.Regexp
	expr    string // original expression
	matcher matcher
}

// String returns the source text used to compile the regular expression.
func (re *Regexp) String() string {
	return re.expr
}

// Compile parses a regular expression and returns, if successful,
// a Regexp object that can be used to match against lines of text.
func Compile(expr string) (*Regexp, error) {
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil, err
	}
	sre := re.Simplify()
	prog, err := syntax.Compile(sre)
	if err != nil {
		return nil, err
	}
	if err := toByteProg(prog); err != nil {
		return nil, err
	}
	r := &Regexp{
		Syntax: re,
		expr:   expr,
	}
	if err := r.matcher.init(prog); err != nil {
		return nil, err
	}
	return r, nil
}

func (re *Regexp) Match(b []byte, beginText, endText bool) (end int) {
	return re.matcher.match(b, beginText, endText)
}

func (re *Regexp) MatchString(s string, beginText, endText bool) (end int) {
	return re.matcher.matchString(s, beginText, endText)
}
