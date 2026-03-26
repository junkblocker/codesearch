# Code Search

A fork of [Google Code Search](https://github.com/google/codesearch) — fast regular expression search over large codebases using a trigram index. Extended with match limits, symlink control, file exclusion, brute-force mode, context lines, a web UI, and a 64-bit index format.

## Quick Start

```shell
# Install (Go 1.16+)
go install github.com/junkblocker/codesearch/cmd/...@latest

# Index your code
cindex $HOME/src /usr/include

# Search
csearch 'func.*Open'
```

## Build

```shell
make all        # lint, vet, test, build, install
make test       # tests with race detection
make release    # cross-compile for all platforms
```

## Binaries

| Command | Purpose |
|---------|---------|
| `cindex` | Build/update the trigram index |
| `csearch` | Search indexed files by regexp |
| `cgrep` | Direct regexp grep (no index) |
| `csweb` | Web search UI (localhost:2473) |

## Documentation

Full structured documentation is in the sibling [`codesearch-docs`](../codesearch-docs/) repository.

## Releases

[github.com/junkblocker/codesearch/releases](https://github.com/junkblocker/codesearch/releases)

## License

BSD-style. See [LICENSE](LICENSE).

## Background

[Regular Expression Matching with a Trigram Index](http://swtch.com/~rsc/regexp/regexp4.html) by Russ Cox.
