// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2025 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"

	"github.com/junkblocker/codesearch/regexp"
)

var usageMessage = `usage: cgrep [-c] [-h] [-i] [-l [-0]] [-n] regexp [file...]

cgrep behaves like grep, searching for regexp, an RE2 (nearly PCRE) regular expression.

Options:
  -c           print only a count of selected lines to stdout
  -h           print this help text and exit
  -i           case-insensitive grep
  -l           print only the names of the files containing matches
  -0           print -l matches separated by NUL ('\0') character
  -n           print each output line preceded by its relative line number in
               the file, starting at 1

Note that as per Go's flag parsing convention, the options cannot be combined.
For example, the option pair -i -n cannot be abbreviated to -in.

The -0 flag is only meaningful with the -l option. It outputs the results
separated by NUL ('\0') character instead of the standard NL ('\n') character.
`

func usage() {
	fmt.Fprint(os.Stderr, usageMessage)
	os.Exit(2)
}

var (
	iflag      = flag.Bool("i", false, "case-insensitive match")
	cpuProfile = flag.String("cpuprofile", "", "write cpu profile to this file")
)

func main() {
	var g regexp.Grep
	g.AddFlags()
	g.Stdout = os.Stdout
	g.Stderr = os.Stderr
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if perr := pprof.StartCPUProfile(f); perr != nil {
			log.Fatal(perr)
		}
		defer pprof.StopCPUProfile()
	}

	pat := "(?m)" + args[0]
	if *iflag {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		log.Fatal(err)
	}
	g.Regexp = re
	if len(args) == 1 {
		g.Reader(os.Stdin, "<standard input>")
	} else {
		for _, arg := range args[1:] {
			g.File(arg)
		}
	}
	if !g.Match {
		os.Exit(1)
	}
}
