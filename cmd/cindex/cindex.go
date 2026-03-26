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
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"

	"github.com/junkblocker/codesearch/index"
)

var usageMessage = `usage: cindex [options] [path...]

Options:

  -verbose     print extra information
  -list        list indexed paths and exit
  -reset       discard existing index
  -indexpath FILE
               use specified FILE as the index path. Overrides $CSEARCHINDEX.
  -cpuprofile FILE
               write CPU profile to FILE
  -logskip     print why a file was skipped from indexing
  -no-follow-symlinks
               do not follow symlinked files and directories
  -maxfilelen BYTES
               skip indexing a file if longer than this size in bytes (Default: %v)
  -maxlinelen BYTES
               skip indexing a file if it has a line longer than this size in bytes (Default: %v)
  -maxtrigrams COUNT
               skip indexing a file if it has more than this number of trigrams (Default: %v)
  -maxinvalidutf8ratio RATIO
               skip indexing a file if it has more than this ratio of invalid UTF-8 sequences (Default: %v)
  -exclude FILE
               path to file containing a list of file patterns to exclude from indexing
  -filelist FILE
               path to file containing a list of file paths to index
  -zip         index content in zip files
  -check       check index is well-formatted

cindex prepares the trigram index for use by csearch.  The index is the
file named by $CSEARCHINDEX, or else $HOME/.csearchindex.

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

By default cindex adds the named paths to the index but preserves
information about other paths that might already be indexed
(the ones printed by cindex -list).  The -reset flag causes cindex to
delete the existing index before indexing the new paths.
With no path arguments, cindex -reset removes the index.
`

func usage() {
	fmt.Fprintf(os.Stderr, usageMessage,
		index.DefaultMaxFileLen, index.DefaultMaxLineLen,
		index.DefaultMaxTextTrigrams, index.DefaultMaxInvalidUTF8Ratio)
	os.Exit(2)
}

var (
	listFlag             = flag.Bool("list", false, "list indexed paths and exit")
	resetFlag            = flag.Bool("reset", false, "discard existing index")
	verboseFlag          = flag.Bool("verbose", false, "print extra information")
	cpuProfile           = flag.String("cpuprofile", "", "write cpu profile to this file")
	checkFlag            = flag.Bool("check", false, "check index is well-formatted")
	indexPath            = flag.String("indexpath", "", "specifies index path")
	logSkipFlag          = flag.Bool("logskip", false, "print why a file was skipped from indexing")
	noFollowSymlinksFlag = flag.Bool("no-follow-symlinks", false, "do not follow symlinked files and directories")
	zipFlag              = flag.Bool("zip", false, "index content in zip files")
	statsFlag            = flag.Bool("stats", false, "print index size statistics")
	exclude              = flag.String("exclude", "", "path to file containing a list of file patterns to exclude from indexing")
	fileList             = flag.String("filelist", "", "path to file containing a list of file paths to index")
	maxFileLen           = flag.Int64("maxfilelen", index.DefaultMaxFileLen, "skip indexing a file if longer than this size in bytes")
	maxLineLen           = flag.Int("maxlinelen", index.DefaultMaxLineLen, "skip indexing a file if it has a line longer than this size in bytes")
	maxTextTrigrams      = flag.Int("maxtrigrams", index.DefaultMaxTextTrigrams, "skip indexing a file if it has more than this number of trigrams")
	maxInvalidUTF8Ratio  = flag.Float64("maxinvalidutf8ratio", index.DefaultMaxInvalidUTF8Ratio, "skip indexing a file if it has more than this ratio of invalid UTF-8 sequences")

	excludePatterns = []string{
		".csearchindex",
	}
)

func walk(arg string, symlinkFrom string, out chan string, logskip bool) {
	err := filepath.Walk(arg, func(path string, info os.FileInfo, err error) error {
		if basedir, elem := filepath.Split(path); elem != "" {
			excluded := false
			for _, pattern := range excludePatterns {
				excluded, err = filepath.Match(pattern, elem)
				if err != nil {
					log.Fatal(err)
				}
				if excluded {
					break
				}
			}

			if info != nil && info.IsDir() {
				if excluded {
					if logskip {
						if symlinkFrom != "" {
							log.Printf("%s: skipped. Excluded directory", symlinkFrom+path[len(arg):])
						} else {
							log.Printf("%s: skipped. Excluded directory", path)
						}
					}
					return filepath.SkipDir
				}
			} else {
				if excluded {
					if logskip {
						if symlinkFrom != "" {
							log.Printf("%s: skipped. Excluded file", symlinkFrom+path[len(arg):])
						} else {
							log.Printf("%s: skipped. Excluded file", path)
						}
					}
					return nil
				}
				if info != nil && info.Mode()&os.ModeSymlink != 0 {
					if *noFollowSymlinksFlag {
						if logskip {
							log.Printf("%s: skipped. Symlink", path)
						}
						return nil
					}
					var symlinkAs string
					if len(basedir) > 0 && basedir[len(basedir)-1] == os.PathSeparator {
						symlinkAs = basedir + elem
					} else {
						symlinkAs = basedir + string(os.PathSeparator) + elem
					}
					if symlinkFrom != "" {
						symlinkAs = symlinkFrom + symlinkAs[len(arg):]
					}
					if p, err := filepath.EvalSymlinks(symlinkAs); err != nil {
						if symlinkFrom != "" {
							log.Printf("%s: skipped. Symlink could not be resolved", symlinkFrom+path[len(arg):])
						} else {
							log.Printf("%s: skipped. Symlink could not be resolved", path)
						}
					} else {
						walk(p, symlinkAs, out, logskip)
					}
					return nil
				}
			}
		}
		if err != nil {
			if symlinkFrom != "" {
				log.Printf("%s: skipped. Error: %s", symlinkFrom+path[len(arg):], err)
			} else {
				log.Printf("%s: skipped. Error: %s", path, err)
			}
			return nil
		}
		if info != nil {
			if info.Mode()&os.ModeType == 0 {
				if symlinkFrom == "" {
					out <- path
				} else {
					out <- symlinkFrom + path[len(arg):]
				}
			} else if !info.IsDir() {
				if logskip {
					if symlinkFrom != "" {
						log.Printf("%s: skipped. Unsupported path type", symlinkFrom+path[len(arg):])
					} else {
						log.Printf("%s: skipped. Unsupported path type", path)
					}
				}
			}
		} else {
			if logskip {
				if symlinkFrom != "" {
					log.Printf("%s: skipped. Could not stat.", symlinkFrom+path[len(arg):])
				} else {
					log.Printf("%s: skipped. Could not stat.", path)
				}
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	log.SetPrefix("cindex: ")
	flag.Usage = usage
	flag.Parse()
	args := flag.Args()

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
		defer ix.Close()
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
		if perr := pprof.StartCPUProfile(f); perr != nil {
			log.Fatal(perr)
		}
		defer pprof.StopCPUProfile()
	}

	if *resetFlag && len(args) == 0 {
		master := index.File()
		stat, err := os.Stat(master)
		if err != nil {
			// does not exist so nothing to do
			return
		}
		if stat != nil && !stat.IsDir() && stat.Mode().IsRegular() {
			os.Remove(master)
			return
		}
		log.Fatal("Invalid index path " + master)
	}

	if *exclude != "" {
		var excludePath string
		if len(*exclude) >= 2 && (*exclude)[:2] == "~/" {
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
		for _, pattern := range strings.Split(string(data), "\n") {
			pattern = strings.TrimSpace(pattern)
			if pattern != "" {
				excludePatterns = append(excludePatterns, pattern)
			}
		}
	}

	if *fileList != "" {
		var fileListPath string
		if len(*fileList) >= 2 && (*fileList)[:2] == "~/" {
			fileListPath = filepath.Join(index.HomeDir(), (*fileList)[2:])
		} else {
			fileListPath = *fileList
		}
		if *logSkipFlag {
			log.Printf("Loading fileList from %s", fileListPath)
		}
		data, err := os.ReadFile(fileListPath)
		if err != nil {
			log.Fatal(err)
		}
		args = append(args, strings.Split(string(data), "\n")...)
	}

	var roots []index.Path
	if len(args) == 0 {
		ix := index.Open(index.File())
		roots = slices.Collect(ix.Roots().All())
		ix.Close()
	} else {
		for _, arg := range args {
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
			ix.Close()
		}
	}

	ix := index.Create(file)
	ix.Verbose = *verboseFlag
	ix.Zip = *zipFlag
	ix.LogSkip = *logSkipFlag
	ix.MaxFileLen = *maxFileLen
	ix.MaxLineLen = *maxLineLen
	ix.MaxTextTrigrams = *maxTextTrigrams
	ix.MaxInvalidUTF8Ratio = *maxInvalidUTF8Ratio
	ix.AddRoots(roots)

	walkChan := make(chan string)
	doneChan := make(chan bool)

	go func() {
		seen := make(map[string]bool)
		for {
			select {
			case path := <-walkChan:
				if !seen[path] {
					seen[path] = true
					if err := ix.AddFile(path); err != nil {
						log.Printf("%s: %s", path, err)
					}
				}
			case <-doneChan:
				return
			}
		}
	}()
	for _, root := range roots {
		log.Printf("index %s", root)
		walk(root.String(), "", walkChan, *logSkipFlag)
	}
	doneChan <- true
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
			ix.Close()
		}
		os.Remove(file)
		os.Rename(file+"~", master)
	} else {
		if *checkFlag {
			ix := index.Open(file)
			if err := ix.Check(); err != nil {
				log.Fatal(err)
			}
			ix.Close()
		}
	}

	log.Printf("done")

	if *statsFlag {
		ix := index.Open(master)
		ix.PrintStats()
		ix.Close()
	}
}
