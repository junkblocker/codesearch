# A fork of Google Code Search to  make it a more generally usable tool

## To install this fork

    cd /
    GO111MODULE=on go get -u github.com/junkblocker/codesearch/cmd/...

## Prebuilt binaries

New releases [https://github.com/junkblocker/codesearch/releases](https://github.com/junkblocker/codesearch/releases)

Old releases [https://github.com/junkblocker/codesearch-pre-github/releases](https://github.com/junkblocker/codesearch-pre-github/releases)

### Old fork pre-"Google on Github" days

[https://github.com/junkblocker/codesearch-pre-github](https://github.com/junkblocker/codesearch-pre-github)

## Original Google codesearch README content

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
