ifneq (,)
This makefile requires GNU Make.
endif

.ONESHELL: ## Make sure same shell is used for all targets for easier passing of settings

BUILDS_DIR = builds

export PROJECT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

RELEASE = $(shell git tag -l | tail -1)

.PHONY: all
help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo "Release is set to $(RELEASE)"

.PHONY: all
all: lint vet test build install ## lint test and install locally
	@echo "Run $(MAKE) publish to publish to github"

.PHONY: release
release: tagcheck deps
	@echo "Building $(RELEASE)"
	@cd "$(PROJECT_DIR)" && goxc -bc="!plan9" -arch='amd64' -pv="$(RELEASE)" -d="$(BUILDS_DIR)" -include=LICENSE -os='darwin freebsd linux windows' go-vet go-test xc archive-zip archive-tar-gz

.PHONY: tagcheck
tagcheck:
	@if [ -z "$(RELEASE)" ]; then \
		echo "Could not determine tag to use. Aborting." ; \
		exit 1 ; \
	fi

.PHONY: deps
deps:
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "GITHUB_TOKEN is not set in the environment" ; \
		exit 1 ; \
	fi
	@if [ -z "$(command -v goxc)" ]; then \
		cd / && go install github.com/laher/goxc@latest ; \
	fi
	@if [ -z "$(command -v ghr)" ]; then \
		cd / && go install github.com/tcnksm/ghr@latest ; \
	fi

.PHONY: publish
publish: release ## Publish a draft release to github
	@echo "Publishing $(RELEASE) draft avoiding overwriting any older existing $(RELEASE) release"
	@echo "Use $(MAKE) publish-force to force publish a non-draft $(RELEASE) release"
	@cd "$(PROJECT_DIR)" && ghr -soft -draft "$(RELEASE)" "$(BUILDS_DIR)/$(RELEASE)/"

.PHONY: publish-force
publish-force: deps ## Force publish to github
	@echo "Force publishing $(RELEASE) to github"
	@cd "$(PROJECT_DIR)" && ghr "$(RELEASE)" "$(BUILDS_DIR)/$(RELEASE)/"

.PHONY: build
build:
	@cd "$(PROJECT_DIR)" && go build ./...

.PHONY: lint
lint:
	@if [ -z "$(command -v golangci-lint)" ]; then \
		cd / && go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.54.2 ; \
	fi
	@cd "$(PROJECT_DIR)" && golangci-lint run --disable=unused,deadcode,ineffassign,gosimple,errcheck,structcheck,varcheck,staticcheck

.PHONY: vet
vet:
	@cd "$(PROJECT_DIR)" && go vet ./...

.PHONY: test
test:
	@cd "$(PROJECT_DIR)" && go test -cover -race ./...

.PHONY: install
install:
	@cd "$(PROJECT_DIR)" && go install ./...

.PHONY: clean
clean: ## Clean the repo
	rm -rf "$(BUILDS_DIR)"
