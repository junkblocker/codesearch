// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2023 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
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

	"github.com/junkblocker/codesearch/index"
	"github.com/junkblocker/codesearch/regexp"
)

var usageMessage = `usage: csearch [options] regexp

Options:

  -c           print only a count of selected lines to stdout
               (Not meaningful with -l or -M modes)
  -f PATHREGEXP
               search only files with names matching this regexp
  -h           print this help text and exit
  -i           case-insensitive search
  -l           print only the names of the files containing matches
               (Not meaningful with -c or -M modes)
  -0           print -l matches separated by NUL ('\0') character
  -m MAXCOUNT  limit search output results to MAXCOUNT (0: no limit)
  -M MAXCOUNT  limit search output results to MAXCOUNT per file (0: no limit)
               (Not allowed with -c or -l modes)
  -n           print each output line preceded by its relative line number in
               the file, starting at 1
  -indexpath FILE
               use specified FILE as the index path. Overrides $CSEARCHINDEX.
  -verbose     print extra information
  -brute       brute force - search all files in index
  -cpuprofile FILE
               write CPU profile to FILE

As per Go's flag parsing convention, the flags cannot be combined: the option
pair -i -n cannot be abbreviated to -in.

csearch behaves like grep over all indexed files, searching for regexp,
an RE2 (nearly PCRE) regular expression.

Csearch relies on the existence of an up-to-date index created ahead of time.
To build or rebuild the index that csearch uses, run:

	cindex path...

where path... is a list of directories or individual files to be included in
the index. If no index exists, this command creates one.  If an index already
exists, cindex overwrites it.  Run cindex -help for more.

csearch uses the index stored in $CSEARCHINDEX or, if that variable is unset or
empty, $HOME/.csearchindex.
`

func usage() {
	fmt.Fprint(os.Stderr, usageMessage)
	os.Exit(2)
}

var (
	fFlag           = flag.String("f", "", "search only files with names matching this regexp")
	iFlag           = flag.Bool("i", false, "case-insensitive search")
	verboseFlag     = flag.Bool("verbose", false, "print extra information")
	bruteFlag       = flag.Bool("brute", false, "brute force - search all files in index")
	cpuProfile      = flag.String("cpuprofile", "", "write cpu profile to this file")
	indexPath       = flag.String("indexpath", "", "specifies index path")
	maxCount        = flag.Int64("m", 0, "specified maximum number of search results")
	maxCountPerFile = flag.Int64("M", 0, "specified maximum number of search results per file")

	matches bool
)

func Main() {
	g := regexp.Grep{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	g.AddFlags()

	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

	if len(args) != 1 || (g.L && g.C) || (g.L && *maxCountPerFile > 0) || (g.C && *maxCountPerFile > 0) {
		usage()
	}

	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *indexPath != "" {
		err := os.Setenv("CSEARCHINDEX", *indexPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	pat := "(?m)" + args[0]
	if *iFlag {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		log.Fatal(err)
	}
	g.Regexp = re
	var fre *regexp.Regexp
	if *fFlag != "" {
		fre, err = regexp.Compile(*fFlag)
		if err != nil {
			log.Fatal(err)
		}
	}
	q := index.RegexpQuery(re.Syntax)
	if *verboseFlag {
		log.Printf("query: %s\n", q)
	}

	ix := index.Open(index.File())
	ix.Verbose = *verboseFlag
	var post []uint32
	if *bruteFlag {
		post = ix.PostingQuery(&index.Query{Op: index.QAll})
	} else {
		post = ix.PostingQuery(q)
	}
	if *verboseFlag {
		log.Printf("post query identified %d possible files\n", len(post))
	}

	if fre != nil {
		fnames := make([]uint32, 0, len(post))

		for _, fileid := range post {
			name := ix.Name(fileid)
			if fre.MatchString(name, true, true) < 0 {
				continue
			}
			fnames = append(fnames, fileid)
		}

		if *verboseFlag {
			log.Printf("filename regexp matched %d files\n", len(fnames))
		}
		post = fnames
	}

	g.LimitPrintCount(*maxCount, *maxCountPerFile)

	for _, fileid := range post {
		name := ix.Name(fileid)
		g.File(name)
		// short circuit here too
		if g.Done {
			break
		}
	}

	matches = g.Match
}

func main() {
	Main()
	if !matches {
		os.Exit(1)
	}
	os.Exit(0)
}
