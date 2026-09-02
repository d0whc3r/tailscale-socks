BIN     := tailscale-socks
PKG     := ./cmd/tailscale-socks
MODULE  := $(shell go list -m)
DIST    := dist
COVER   := coverage

# Modules behind the go.mod `tool` block. Lazy `=`: only `make outdated` pays
# for the extra `go list`.
TOOL_MODULES = $(shell go list -f '{{.Module.Path}}' tool | sort -u)
OUTDATED_FMT = {{if .Update}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}

HOST_OS   := $(shell go env GOHOSTOS)
HOST_ARCH := $(shell go env GOHOSTARCH)

# Target platform for `make build`. Defaults to the host; `all` means every
# matching entry of PLATFORMS, so OS=all ARCH=arm64 builds every arm64 target.
OS   ?= $(HOST_OS)
ARCH ?= $(HOST_ARCH)

# Release matrix: what `OS=all`/`ARCH=all` expands to.
PLATFORMS := \
	darwin/amd64 darwin/arm64 \
	linux/amd64 linux/arm64 linux/arm \
	windows/amd64 windows/arm64

.PHONY: all build run test test-sh vet fmt fmt-check lint check cover cover-html vuln outdated release tidy hooks clean

all: hooks check build

# A plain host build goes to ./$(BIN) unstripped, so it stays debuggable.
# Anything cross-compiled or expanded from `all` goes to $(DIST)/ as a static,
# stripped release artifact.
build:
	@set -e; \
	if [ "$(OS)" = all ] || [ "$(ARCH)" = all ]; then \
		targets=""; \
		for p in $(PLATFORMS); do \
			os=$${p%/*}; arch=$${p#*/}; \
			{ [ "$(OS)" = all ] || [ "$(OS)" = "$$os" ]; } || continue; \
			{ [ "$(ARCH)" = all ] || [ "$(ARCH)" = "$$arch" ]; } || continue; \
			targets="$$targets $$os/$$arch"; \
		done; \
		[ -n "$$targets" ] || { \
			echo "no PLATFORMS entry matches OS=$(OS) ARCH=$(ARCH)"; \
			echo "PLATFORMS: $(PLATFORMS)"; exit 1; }; \
	else \
		targets="$(OS)/$(ARCH)"; \
	fi; \
	for p in $$targets; do \
		os=$${p%/*}; arch=$${p#*/}; \
		ext=""; [ "$$os" = windows ] && ext=".exe"; \
		if [ "$$p" = "$(HOST_OS)/$(HOST_ARCH)" ] && [ "$(OS)" != all ] && [ "$(ARCH)" != all ]; then \
			echo "build $(BIN)$$ext (host)"; \
			go build -o $(BIN)$$ext $(PKG); \
		else \
			mkdir -p $(DIST); \
			out=$(DIST)/$(BIN)-$$os-$$arch$$ext; \
			echo "build $$out"; \
			CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
				go build -trimpath -ldflags="-s -w" -o $$out $(PKG); \
		fi; \
	done

run: build
	./$(BIN)

# Release artifacts are GoReleaser's job, so the matrix lives in one place
# (.goreleaser.yaml): this is the local dry run, CI runs the real thing on a tag
# push. Nothing in it needs a Mac, which is why CI releases from ubuntu.
# Cross-building without GoReleaser still works: make build OS=all ARCH=all
release:
	@command -v goreleaser >/dev/null 2>&1 || \
		{ echo "goreleaser not installed: brew install goreleaser"; exit 1; }
	goreleaser release --snapshot --clean

# -race because every listener runs in its own goroutine.
test:
	go test -race ./...

vet:
	go vet ./...

# goimports, staticcheck and govulncheck are pinned in the go.mod `tool`
# block, so `go tool` runs the exact versions with no separate install step.

# The module's own Go, and only that: `.` would also walk the Go files under
# .agents/, which belong to a vendored skill and are not ours to rewrite.
GOSRC := ./cmd ./internal

# Rewrite files in place: gofmt plus import fixing/grouping.
fmt:
	go tool goimports -w -local $(MODULE) $(GOSRC)

# Same check, read-only. Fails listing what `make fmt` would change.
fmt-check:
	@out=$$(go tool goimports -l -local $(MODULE) $(GOSRC)); \
	if [ -n "$$out" ]; then echo "unformatted (run: make fmt):"; echo "$$out"; exit 1; fi

lint: fmt-check vet
	go tool staticcheck ./...

# The zsh helpers get the same treatment as the Go code. `zsh -n` is the only
# linting there is — shellcheck rejects the dialect outright (SC1071) — and
# the suite runs in bats, which needs 1.5.0 or newer for `run --separate-stderr`.
# Every service manager is stubbed, so all three backends are exercised on
# whatever machine runs this: nothing is installed and no node is started.
ZSHFILES := contrib/tailscale-socks.zsh $(wildcard contrib/platform/*.zsh contrib/test/*.zsh)
SHFILES := contrib/install.sh $(wildcard packaging/*.sh .githooks/*)
BATSFILES := $(wildcard contrib/test/*.bats contrib/test/*.bash packaging/test/*.bats)

# bats spawns processes per test and has no cache of its own, which makes it
# an order of magnitude slower than everything else in `check`. So a passing
# run is stamped with a checksum of what it covers, and a Go-only edit does
# not pay for it again. Force a run with `rm -f .tmp/test-sh.cksum`.
#
# Content and not mtimes: the make that ships with macOS is 3.81 and compares
# timestamps to the second, so an edit landing inside the same second as the
# stamp would be missed. `cksum` over the files and not over their contents
# joined, or moving a line from the end of one file to the start of the next
# would leave the sum unchanged.
SHSTAMP := .tmp/test-sh.cksum

# One shell for the whole recipe: make gives each line its own, and the skip
# has to be able to end the run. The sum is computed here rather than in a
# variable so that `make build` does not pay for it.
test-sh:
	@sum=$$(cksum Makefile $(ZSHFILES) $(SHFILES) $(BATSFILES) | cksum); \
	if [ "$$(cat $(SHSTAMP) 2>/dev/null)" = "$$sum" ]; then exit 0; fi; \
	command -v zsh >/dev/null 2>&1 || { echo "zsh not installed"; exit 1; }; \
	command -v bats >/dev/null 2>&1 || \
		{ echo "bats not installed: brew install bats-core"; exit 1; }; \
	for f in $(ZSHFILES); do zsh -n "$$f" || exit 1; done; \
	for f in $(SHFILES); do sh -n "$$f" || exit 1; done; \
	bats --print-output-on-failure contrib/test packaging/test || exit 1; \
	mkdir -p $(dir $(SHSTAMP)) && echo "$$sum" > $(SHSTAMP)

check: lint test test-sh

# Statement coverage, straight from the toolchain. `cover` prints the
# per-function table with the module total on the last line; `cover-html`
# opens the annotated source in a browser.
#
# -race to match `test`, because without it this is a second compile of the
# whole tree into a different cache profile: CI would build tailscale.com twice
# over to learn nothing new. The count comes out identical either way, so
# `make cover` alone is the full test run plus the profile.
COVERPROFILE := $(COVER)/cover.out

cover:
	@mkdir -p $(COVER)
	go test -race -coverprofile=$(COVERPROFILE) ./...
	@go tool cover -func=$(COVERPROFILE)

cover-html: cover
	go tool cover -html=$(COVERPROFILE)

# Known vulnerabilities in the dependency tree. Kept out of `check`: it needs
# the network and the vulnerability database.
vuln:
	go tool govulncheck ./...

# Direct dependencies and pinned tools with a newer version. The `all` pattern
# walks the whole transitive graph (tailscale.com alone pulls in ~1000
# modules), so indirect entries are dropped; the tools are queried by module
# path because go.mod records them as indirect too.
outdated:
	@out=$$( \
		go list -m -u -f '{{if not .Indirect}}$(OUTDATED_FMT){{end}}' all; \
		go list -m -u -f '$(OUTDATED_FMT)' $(TOOL_MODULES)); \
	if [ -n "$$out" ]; then echo "$$out"; else echo "all dependencies current"; fi

tidy:
	go mod tidy

# Point git at the tracked hooks in .githooks/, so `git commit` runs the same
# gate as CI. One command per clone; `git commit --no-verify` skips it once.
hooks:
	@git config core.hooksPath .githooks
	@echo "git hooks enabled: .githooks (skip once with git commit --no-verify)"

clean:
	rm -rf $(BIN) $(BIN).exe $(DIST) $(COVER)
