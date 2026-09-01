BIN     := tailscale-socks
PKG     := ./cmd/tailscale-socks
MODULE  := $(shell go list -m)
DIST    := dist

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

.PHONY: all build run test vet fmt fmt-check lint check release tidy clean

all: check build

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

release:
	@$(MAKE) build OS=all ARCH=all

test:
	go test ./...

vet:
	go vet ./...

# goimports and staticcheck are pinned in the go.mod `tool` block, so
# `go tool` runs the exact versions with no separate install step.

# Rewrite files in place: gofmt plus import fixing/grouping.
fmt:
	go tool goimports -w -local $(MODULE) .

# Same check, read-only. Fails listing what `make fmt` would change.
fmt-check:
	@out=$$(go tool goimports -l -local $(MODULE) .); \
	if [ -n "$$out" ]; then echo "unformatted (run: make fmt):"; echo "$$out"; exit 1; fi

lint: fmt-check vet
	go tool staticcheck ./...

check: lint test

tidy:
	go mod tidy

clean:
	rm -rf $(BIN) $(BIN).exe $(DIST)
