# A fork of Google Code Search to  make it a more generally usable tool

## Prebuilt binaries for this fork

* New releases [https://github.com/junkblocker/codesearch/releases](https://github.com/junkblocker/codesearch/releases)

## Installing this fork from source

With Go 1.16 or newer

``` shell
cd /
go install github.com/junkblocker/codesearch/cmd/...@latest
```

## Historical content

* Old releases [https://github.com/junkblocker/codesearch-pre-github/releases](https://github.com/junkblocker/codesearch-pre-github/releases)

* Old fork pre-"Google on Github" days [https://github.com/junkblocker/codesearch-pre-github](https://github.com/junkblocker/codesearch-pre-github)

* Original Google codesearch README extract

``` text
Code Search is a tool for indexing and then performing
regular expression searches over large bodies of source code.
It is a set of command-line programs written in Go.
Binary downloads are available for those who do not have Go installed.
See https://github.com/google/codesearch.

For background and an overview of the commands,
see http://swtch.com/~rsc/regexp/regexp4.html.

To install:

    go get github.com/google/codesearch/cmd/...

Use "go get -u" to update an existing installation.

Russ Cox
rsc@swtch.com
June 2015
```
