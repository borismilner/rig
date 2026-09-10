# rig - the platform every in-house program runs on.
# `make help` lists every target. Nothing important happens outside this file.

BIN        := rig
MODULE     := github.com/boris-milner/rig
PREFIX     ?= $(HOME)/.local
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
WIRE       := v1
SHA        := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
              -X main.version=$(VERSION) -X main.wire=$(WIRE) \
              -X main.sha=$(SHA)         -X main.date=$(DATE)
GOFLAGS    := -trimpath
COVER_MIN  := 90
COVER_OUT  := build/cover.out
CHAOS_ITER ?= 10000
FUZZ_TIME  ?= 60s

.DEFAULT_GOAL := help
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c

##@ Build

build: ## Build the rig binary into build/
	@mkdir -p build
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/$(BIN) ./cmd/rig

build-all: ## Cross-compile for every supported target
	@mkdir -p build
	@for t in linux/amd64 linux/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "  $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
	    -o build/$(BIN)-$$os-$$arch ./cmd/rig; \
	done

install: build ## Install rig to $(PREFIX)/bin and register the user service
	install -Dm755 build/$(BIN) $(PREFIX)/bin/$(BIN)
	@echo "installed $(PREFIX)/bin/$(BIN) ($(VERSION))"

uninstall: ## Remove the installed binary and the user service
	rm -f $(PREFIX)/bin/$(BIN)

run: build ## Run the daemon in the foreground with debug logging
	./build/$(BIN) serve --log-level=debug

dev: ## Run the daemon with reload on save (needs air)
	@command -v air >/dev/null || { echo "air not found: go install github.com/air-verse/air@latest"; exit 1; }
	air -c .air.toml

clean: ## Remove build output, coverage and generated artefacts
	rm -rf build dist

##@ Test

test: test-unit test-race ## Run the standard test suite (unit + race)

test-unit: ## Run unit tests
	go test ./...

test-race: ## Run tests under the race detector
	go test -race ./...

test-chaos: ## Kill, hang, flood and restart fakeapp $(CHAOS_ITER) times
	go test -run TestChaos -count=1 -timeout 30m ./internal/supervise/... -args -iterations=$(CHAOS_ITER)

test-e2e: ## Drive the real window and TUI (Playwright + teatest)
	go test -tags=e2e ./test/e2e/...
	cd frontend && npm run test:e2e

test-wire: ## Golden-wire: last release's binary against HEAD
	go test -run TestGoldenWire -count=1 ./internal/wire/...

fuzz: ## Fuzz the registration parser, schemas and frame codec for $(FUZZ_TIME) each
	@for t in FuzzRegistration FuzzSchema FuzzFrame; do \
	  echo "  $$t"; go test -run=NONE -fuzz=$$t -fuzztime=$(FUZZ_TIME) ./internal/... ; \
	done

cover: ## Run tests with coverage and fail below $(COVER_MIN)%
	@mkdir -p build
	go test -coverprofile=$(COVER_OUT) -covermode=atomic ./...
	@go tool cover -func=$(COVER_OUT) | tail -1
	@pct=$$(go tool cover -func=$(COVER_OUT) | tail -1 | grep -oE '[0-9.]+' | tail -1); \
	 awk -v p=$$pct -v m=$(COVER_MIN) 'BEGIN{if(p+0<m){printf "coverage %.1f%% is below %d%%\n",p,m; exit 1}}'

cover-html: cover ## Open the coverage report in a browser
	go tool cover -html=$(COVER_OUT)

##@ Quality

lint: ## Run golangci-lint plus the two house analyzers
	golangci-lint run ./...
	go run ./internal/analysis/cmd/noprogramid ./...
	go run ./internal/analysis/cmd/nocontextfree ./...

fmt: ## Format Go and frontend sources
	gofmt -s -w .
	go run golang.org/x/tools/cmd/goimports@latest -w .
	cd frontend && npm run format

vet: ## go vet
	go vet ./...

audit: ## Check dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

verify: build ## Run the conformance suite against fakeapp, then against every misbehaviour
	./build/$(BIN) verify ./build/fakeapp
	@for m in hang crash leak flood lie ignore-cancel garbage; do \
	  echo "  --misbehave=$$m must fail"; \
	  ./build/$(BIN) verify ./build/fakeapp --misbehave=$$m && { echo "  did not fail"; exit 1; } || true; \
	done

contrast: ## Measure WCAG contrast in a real browser, both themes
	node tools/contrast-audit.mjs --themes=light,dark --fail-under=4.5

##@ Generate

generate: proto schema types docs ## Regenerate everything that is generated

proto: ## Generate Go and TypeScript from proto/
	buf generate

schema: ## Emit JSON Schema from the Go declaration types
	go run ./cmd/schemagen -out schema/

types: schema ## Generate TypeScript types from the JSON Schemas
	cd frontend && npm run gen:types

docs: ## Regenerate the CLI reference and the capability map
	go run ./cmd/rig docs --out docs/cli.md

##@ Measure

bench: ## Run Go benchmarks
	go test -bench=. -benchmem -run=NONE ./...

bench-ipc: ## Reproduce the transport numbers in PLAN.md section 4
	cd ipcbench && go build -o ../build/ipcbench . && ../build/ipcbench

bench-idle: build ## Measure idle footprint against the budget in PLAN.md section 13
	go run ./cmd/footprint --binary build/$(BIN) --quiet-for 60s \
	  --max-rss 20MiB --max-cpu 0.1 --max-wakeups 1

bench-scale: build ## Measure the per-program overhead with 1, 10 and 50 programs registered
	go run ./cmd/footprint --binary build/$(BIN) --programs 1,10,50 --max-delta 500KiB

build-minimal: ## Build the kernel-only daemon, no services, no surfaces
	@mkdir -p build
	go build $(GOFLAGS) -tags minimal -ldflags '$(LDFLAGS)' -o build/$(BIN)-minimal ./cmd/rig
	@ls -la build/$(BIN)-minimal

modules: ## List every service, surface and renderer, and prove none imports another
	go run ./cmd/rig modules list
	go run ./internal/analysis/cmd/nomodulecross ./...

profile: build ## Capture a CPU profile of the daemon under load
	./build/$(BIN) serve --pprof=127.0.0.1:6060 & \
	 sleep 2; go tool pprof -http=: http://127.0.0.1:6060/debug/pprof/profile?seconds=30

##@ Operate

up: build ## Start the daemon in the background
	./build/$(BIN) up

down: ## Stop the daemon
	./build/$(BIN) down

doctor: ## Report the health of the whole installation
	./build/$(BIN) doctor

apps: ## List registered programs and their state
	./build/$(BIN) apps list

logs: ## Follow the merged log stream
	./build/$(BIN) logs --follow

tui: build ## Open the terminal client
	./build/$(BIN) tui

##@ Ship

tidy: ## Tidy go.mod and npm dependencies
	go mod tidy
	cd frontend && npm prune

deps-check: ## Fail if any pinned dependency has drifted from upstream
	go run ./cmd/depscheck --plan PLAN.md

release: ci ## Tag, generate the changelog and build release artefacts
	@test -n "$(V)" || { echo "usage: make release V=1.2.3"; exit 1; }
	git tag -a v$(V) -m "release v$(V)"
	git cliff --tag v$(V) -o CHANGELOG.md
	git add CHANGELOG.md && git commit -m "chore(release): v$(V)"
	$(MAKE) build-all package

package: ## Build the .deb
	@mkdir -p dist
	go run ./cmd/pkgdeb --version $(VERSION) --out dist/

ci: fmt-check lint vet audit modules cover test-race test-wire verify bench-idle ## Everything CI runs

fmt-check: ## Fail if anything is unformatted
	@out=$$(gofmt -s -l .); test -z "$$out" || { echo "unformatted:"; echo "$$out"; exit 1; }

##@ Meta

version: ## Print every version this build carries
	@echo "product $(VERSION)"
	@echo "wire    $(WIRE)"
	@echo "commit  $(SHA)"
	@echo "built   $(DATE)"

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nrig $(VERSION)\n\nusage: make <target>\n"} \
	  /^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5)} \
	  /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo

.PHONY: build build-all install uninstall run dev clean test test-unit test-race \
        test-chaos test-e2e test-wire fuzz cover cover-html lint fmt vet audit \
        verify contrast generate proto schema types docs bench bench-ipc profile \
        up down doctor apps logs tui tidy deps-check release package ci fmt-check \
        bench-idle bench-scale build-minimal modules version help
