BUILDS_DIR = builds

RELEASE = $(shell git tag -l | tail -1 )

.PHONY: all
help: ## Prints help for targets with comments
	@cat $(MAKEFILE_LIST) | grep -E '^[a-zA-Z_-]+:.*?## .*$$' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
	@echo "Release is set to $(RELEASE)"

.PHONY: all
all: lint vet test build install ## lint test and install locally
	@echo "Run $(MAKE) publish to publish to github"

.PHONY: release
release: tagcheck
	@echo "Building $(RELEASE)"
	mkdir -p /tmp/goxc-tmp-intall && cd /tmp/goxc-tmp-intall && go get github.com/laher/goxc
	rm -rf /tmp/goxc-tmp-intall
	goxc -bc="!plan9" -arch='amd64' -pv="$(RELEASE)" -d="$(BUILDS_DIR)" -include=LICENSE -os='darwin freebsd linux windows' go-vet go-test xc archive-zip archive-tar-gz

.PHONY: tagcheck
tagcheck:
	@if [ -z "$(RELEASE)" ]; then \
		echo "Could not determine tag to use. Aborting." ; \
		fail ; \
	fi

.PHONY: deps
deps: tagcheck
	@if [ -z "$(GITHUB_TOKEN)" ]; then \
		echo "GITHUB_TOKEN is not set in the environment" ; \
		fail ; \
	fi
	mkdir -p /tmp/ghr-tmp-install && cd /tmp/ghr-tmp-install && go get github.com/laher/goxc
	rm -rf /tmp/ghr-tmp-install
	go get -u github.com/tcnksm/ghr

.PHONY: publish
publish: deps ## Publish a draft release to github
	@echo "Publishing $(RELEASE) draft avoiding overwriting any older existing $(RELEASE) release"
	@echo "Use $(MAKE) publish-force to force publish a non-draft $(RELEASE) release"
	@ghr -soft -draft "$(RELEASE)" "$(BUILDS_DIR)/$(RELEASE)/"

.PHONY: publish-force
publish-force: deps ## Force publish to github
	@echo "Force publishing $(RELEASE) to github"
	@ghr "$(RELEASE)" "$(BUILDS_DIR)/$(RELEASE)/"

.PHONY: build
build:
	@go build ./...

.PHONY: lint
lint:
	@golint ./...

.PHONY: vet
vet:
	@go vet ./...

.PHONY: test
test:
	@go test -cover -race ./...

.PHONY: install
install:
	@go install ./...

.PHONY: clean
clean: ## Clean the repo
	rm -rf "$(BUILDS_DIR)"
