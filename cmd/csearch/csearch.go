// Copyright 2011 The Go Authors.  All rights reserved.
// Copyright 2013-2025 Manpreet Singh ( junkblocker@yahoo.com ). All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/junkblocker/codesearch/index"
	"github.com/junkblocker/codesearch/regexp"
)

var usageMessage = `usage: csearch [options] regexp

Options:

  -c           print only a count of selected lines to stdout
               (Not meaningful with -l or -M modes)
  -f FILEREGEXP
               search only files with names matching this RE2 regular expression FILEREGEXP
  -h           omit line numbers in search results
               (Not meaningful with -c, or -0 modes)
  -help        print this help text and exit
  -i           case-insensitive search
  -l           print only the names of the files containing matches
               (Not meaningful with -c, -0 or -M modes)
  -n           print each output line preceded by its relative line number in
               the file, starting at 1
  -indexpath FILE
               use specified FILE as the index path. Overrides $CSEARCHINDEX.
  -verbose     print extra information
  -brute       brute force - search all files in index
  -0           print -l matches separated by NUL ('\0') character
               (Not meaningful with -h, or -c modes)
  -cpuprofile FILE
               write CPU profile to FILE

csearch behaves like grep over all indexed files, searching for regexp,
an RE2 (nearly PCRE) regular expression.

The -c, -h, -i, -l, and -n flags are as in grep, although note that as per Go's
flag parsing convention, they cannot be combined: the option pair -i -n
cannot be abbreviated to -in.

The -f flag restricts the search to files whose names match the RE2 regular
expression fileregexp.

Csearch relies on the existence of an up-to-date index created ahead of time.
To build or rebuild the index that csearch uses, run:

	cindex path...

where path... is a list of directories or individual files to be included in the index.
If no index exists, this command creates one.  If an index already exists, cindex
overwrites it.  Run cindex -help for more.

Csearch uses the index stored in $CSEARCHINDEX or, if that variable is unset or
empty, $HOME/.csearchindex. The -indexpath FILE options uses specified FILE as the
index path overriding these.
`

func usage() {
	fmt.Fprintf(os.Stderr, usageMessage)
	os.Exit(2)
}

var (
	fFlag       = flag.String("f", "", "search only files with names matching this regexp")
	iFlag       = flag.Bool("i", false, "case-insensitive search")
	htmlFlag    = flag.Bool("html", false, "print HTML output")
	verboseFlag = flag.Bool("verbose", false, "print extra information")
	bruteFlag   = flag.Bool("brute", false, "brute force - search all files in index")
	cpuProfile  = flag.String("cpuprofile", "", "write cpu profile to this file")
	indexPath   = flag.String("indexpath", "", "specifies index path")

	matches bool
)

func Main() {
	log.SetPrefix("csearch: ")
	g := regexp.Grep{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	g.AddFlags()

	flag.Usage = usage
	flag.Parse()
	if *htmlFlag {
		g.HTML = true
	}
	args := flag.Args()

	if len(args) != 1 || (g.L && g.C) || (g.Z && g.C) || (g.H && g.C) || (g.Z && g.H) || (g.Z && !g.L) {
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
	var post []int
	if *bruteFlag {
		post = ix.PostingQuery(&index.Query{Op: index.QAll})
	} else {
		post = ix.PostingQuery(q)
	}
	if *verboseFlag {
		log.Printf("post query identified %d possible files\n", len(post))
	}

	if fre != nil {
		fnames := make([]int, 0, len(post))

		for _, fileid := range post {
			name := ix.Name(fileid)
			if fre.MatchString(name.String(), true, true) < 0 {
				continue
			}
			fnames = append(fnames, fileid)
		}

		if *verboseFlag {
			log.Printf("filename regexp matched %d files\n", len(fnames))
		}
		post = fnames
	}

	var (
		zipFile   string
		zipReader *zip.ReadCloser
		zipMap    map[string]*zip.File
	)

	for _, fileid := range post {
		name := ix.Name(fileid).String()
		if g.L && (pat == "(?m)" || pat == "(?i)(?m)") {
			g.Reader(bytes.NewReader(nil), name)
			continue
		}
		file, err := os.Open(string(name))
		if err != nil {
			if i := strings.Index(name, ".zip\x01"); i >= 0 {
				zfile, zname := name[:i+4], name[i+5:]
				if zfile != zipFile {
					if zipReader != nil {
						zipReader.Close()
						zipMap = nil
					}
					zipFile = zfile
					zipReader, err = zip.OpenReader(zfile)
					if err != nil {
						zipReader = nil
					}
					if zipReader != nil {
						zipMap = make(map[string]*zip.File)
						for _, file := range zipReader.File {
							zipMap[file.Name] = file
						}
					}
				}
				file := zipMap[zname]
				if file != nil {
					r, err := file.Open()
					if err != nil {
						continue
					}
					g.Reader(r, name)
					r.Close()
					continue
				}
			}
			continue
		}
		g.Reader(file, name)
		file.Close()
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
