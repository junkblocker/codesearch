ifneq (,)
This makefile requires GNU Make.
endif

.ONESHELL: ## Make sure same shell is used for all targets for easier passing of settings

BUILDS_DIR = builds

export PROJECT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

RELEASE = $(shell git tag -l | tail -1)

# Supported platforms
PLATFORMS = \
    linux-amd64 \
    linux-arm64 \
    darwin-amd64 \
    darwin-arm64 \
    freebsd-amd64 \
    freebsd-arm64 \
    windows-amd64 \
    windows-arm64

EXECUTABLES := csearch cindex cgrep

.PHONY: all
help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo "Release is set to $(RELEASE)"

.PHONY: all
all: lint vet test build install ## lint test and install locally
	@echo "Run $(MAKE) publish to publish to github"

$(EXECUTABLES): deps vet test
	@cd "$(PROJECT_DIR)" && for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'-' -f1); \
		GOARCH=$$(echo $$platform | cut -d'-' -f2); \
		output_name=$@; \
		if [ $$GOOS = "windows" ]; then \
			output_name=$${output_name}.exe; \
		fi; \
		echo "Building $$output_name for $$GOOS/$$GOARCH"; \
		rm -f builds/$(RELEASE)/$${GOOS}_$${GOARCH}/$${output_name}; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build -o builds/$(RELEASE)/$${GOOS}_$${GOARCH}/$${output_name} ./cmd/$@; \
	done

.PHONY: release
release: $(EXECUTABLES)
	@cd "$(PROJECT_DIR)" && for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'-' -f1); \
		GOARCH=$$(echo $$platform | cut -d'-' -f2); \
		echo "Bundling for $$GOOS/$$GOARCH"; \
		cp LICENSE "builds/$(RELEASE)/$${GOOS}_$${GOARCH}/" ; \
		if [ $$GOOS = "windows" ]; then \
			rm -f builds/codesearch_$(RELEASE)_$${GOOS}_$${GOARCH}.zip ; \
			zip -D -r -j builds/$(RELEASE)/codesearch_$(RELEASE)_$${GOOS}_$${GOARCH}.zip builds/$(RELEASE)/$${GOOS}_$${GOARCH}
		else \
			rm -f builds/codesearch_$(RELEASE)_$${GOOS}_$${GOARCH}.tar.gz ; \
			tar -cvzf builds/$(RELEASE)/codesearch_$(RELEASE)_$${GOOS}_$${GOARCH}.tar.gz -C builds/$(RELEASE)/$${GOOS}_$${GOARCH} .
		fi; \
	done



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
