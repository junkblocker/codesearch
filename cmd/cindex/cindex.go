// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"

	"github.com/google/codesearch/index"
)

var usageMessage = `usage: cindex [-list] [-reset] [-zip] [path...]

Cindex prepares the trigram index for use by csearch.  The index is the
file named by $CSEARCHINDEX, or else $HOME/.csearchindex. The -indexpath FILE
options uses specified FILE as the index path overriding these.

The simplest invocation is

	cindex path...

which adds the file or directory tree named by each path to the index.
For example:

	cindex $HOME/src /usr/include

or, equivalently:

	cindex $HOME/src
	cindex /usr/include

If cindex is invoked with no paths, it reindexes the paths that have
already been added, in case the files have changed.  Thus, 'cindex' by
itself is a useful command to run in a nightly cron job.

The -list flag causes cindex to list the paths it has indexed and exit.

The -zip flag causes cindex to index content inside ZIP files.
This feature is experimental and will almost certainly change
in the future, possibly in incompatible ways.

By default cindex adds the named paths to the index but preserves
information about other paths that might already be indexed
(the ones printed by cindex -list).  The -reset flag causes cindex to
delete the existing index before indexing the new paths.
With no path arguments, cindex -reset removes the index.
`

func usage() {
	fmt.Fprintf(os.Stderr, usageMessage)
	os.Exit(2)
}

var (
	listFlag    = flag.Bool("list", false, "list indexed paths and exit")
	resetFlag   = flag.Bool("reset", false, "discard existing index")
	verboseFlag = flag.Bool("verbose", false, "print extra information")
	cpuProfile  = flag.String("cpuprofile", "", "write cpu profile to this file")
	checkFlag   = flag.Bool("check", false, "check index is well-formatted")
	logSkipFlag = flag.Bool("logskip", false, "print why a file was skipped from indexing")
	indexPath   = flag.String("indexpath", "", "specifies index path")
	exclude     = flag.String("exclude", "", "path to file containing a list of file patterns to exclude from indexing")
	zipFlag     = flag.Bool("zip", false, "index content in zip files")
	statsFlag   = flag.Bool("stats", false, "print index size statistics")

	excludePatterns = []string{
		".csearchindex",
	}
)

func main() {
	log.SetPrefix("cindex: ")
	flag.Usage = usage
	flag.Parse()

	if *indexPath != "" {
		if err := os.Setenv("CSEARCHINDEX", *indexPath); err != nil {
			log.Fatal(err)
		}
	}

	if *listFlag {
		master := index.File()
		if stat, err := os.Stat(master); err != nil || stat == nil {
			log.Fatal("Index " + master + " is not accessible")
		} else if stat.IsDir() || !stat.Mode().IsRegular() {
			log.Fatal("Index " + master + " must point to an index file")
		}
		ix := index.Open(master)
		if *checkFlag {
			if err := ix.Check(); err != nil {
				log.Fatal(err)
			}
		}
		for p := range ix.Roots().All() {
			fmt.Printf("%s\n", p)
		}
		return
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

	if *resetFlag && flag.NArg() == 0 {
		master := index.File()
		stat, err := os.Stat(master)
		if err != nil {
			// does not exist so nothing to do
			return
		}
		if stat != nil && !stat.IsDir() && stat.Mode().IsRegular() {
			os.Remove(master)
			return
		} else {
			log.Fatal("Invalid index path " + master)
		}
	}

	if *exclude != "" {
		var excludePath string
		if (*exclude)[:2] == "~/" {
			excludePath = filepath.Join(index.HomeDir(), (*exclude)[2:])
		} else {
			excludePath = *exclude
		}
		if *logSkipFlag {
			log.Printf("Loading exclude patterns from %s", excludePath)
		}
		data, err := os.ReadFile(excludePath)
		if err != nil {
			log.Fatal(err)
		}
		excludePatterns = append(excludePatterns, strings.Split(string(data), "\n")...)
		for i, pattern := range excludePatterns {
			excludePatterns[i] = strings.TrimSpace(pattern)
		}
	}

	var roots []index.Path
	if flag.NArg() == 0 {
		ix := index.Open(index.File())
		roots = slices.Collect(ix.Roots().All())
	} else {
		// Translate arguments to absolute paths so that
		// we can generate the file list in sorted order.
		for _, arg := range flag.Args() {
			a, err := filepath.Abs(arg)
			if err != nil {
				log.Printf("%s: %s", arg, err)
				continue
			}
			roots = append(roots, index.MakePath(a))
		}
		slices.SortFunc(roots, index.Path.Compare)
	}

	master := index.File()
	if stat, err := os.Stat(master); err != nil {
		// Does not exist.
		*resetFlag = true
	} else {
		if stat != nil && (stat.IsDir() || !stat.Mode().IsRegular()) {
			log.Fatal("Invalid index path " + master)
		}
	}
	file := master
	if !*resetFlag {
		file += "~"
		if *checkFlag {
			ix := index.Open(master)
			if err := ix.Check(); err != nil {
				log.Fatal(err)
			}
		}
	}

	ix := index.Create(file)
	ix.Verbose = *verboseFlag
	ix.Zip = *zipFlag
	ix.LogSkip = *logSkipFlag
	ix.AddRoots(roots)
	for _, root := range roots {
		log.Printf("index %s", root)
		filepath.Walk(root.String(), func(path string, info os.FileInfo, err error) error {
			if _, elem := filepath.Split(path); elem != "" {
				exclude := false
				for _, pattern := range excludePatterns {
					exclude, err = filepath.Match(pattern, elem)
					if err != nil {
						log.Fatal(err)
					}
					if exclude {
						break
					}
				}

				// Skip various temporary or "hidden" files or directories.
				if exclude {
					if info != nil && info.IsDir() {
						if *logSkipFlag {
							log.Printf("%s: skipped. Excluded directory", path)
						}
						return filepath.SkipDir
					}
					if *logSkipFlag {
						log.Printf("%s: skipped. Excluded file", path)
					}
					return nil
				}
			}
			if err != nil {
				log.Printf("%s: %s", path, err)
				return nil
			}
			if info != nil && info.Mode()&os.ModeType == 0 {
				if err := ix.AddFile(path); err != nil {
					log.Printf("%s: %s", path, err)
					return nil
				}
			}
			return nil
		})
	}
	log.Printf("flush index")
	ix.Flush()

	if !*resetFlag {
		log.Printf("merge %s %s", master, file)
		index.Merge(file+"~", master, file)
		if *checkFlag {
			ix := index.Open(file + "~")
			if err := ix.Check(); err != nil {
				log.Fatal(err)
			}
		}
		os.Remove(file)
		os.Rename(file+"~", master)
	} else {
		if *checkFlag {
			ix := index.Open(file)
			if err := ix.Check(); err != nil {
				log.Fatal(err)
			}
		}
	}

	log.Printf("done")

	if *statsFlag {
		ix := index.Open(master)
		ix.PrintStats()
	}
	return
}
