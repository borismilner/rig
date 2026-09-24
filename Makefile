# rig - the platform every in-house program runs on.
# `make help` lists every target. Nothing important happens outside this file.

# Two binaries, and the split is load-bearing (section 22): rigd links none of
# the terminal stack, rig links no daemon internals. bench-size attributes a
# dependency's cost to whichever of them actually pays it.
BIN        := rig
BIND       := rigd
MODULE     := github.com/borismilner/rig
PREFIX     ?= $(HOME)/.local
# Where the systemd --user unit lands. systemd reads XDG_CONFIG_HOME
# and falls back to ~/.config, so this follows it rather than guessing.
UNITDIR    ?= $(or $(XDG_CONFIG_HOME),$(HOME)/.config)/systemd/user
# Where a desktop launcher and its icon go, per the XDG basedir spec. Both
# follow XDG_DATA_HOME so a machine that moves it does not get a launcher
# written somewhere nothing reads.
APPDIR     ?= $(or $(XDG_DATA_HOME),$(HOME)/.local/share)/applications
ICONDIR    ?= $(or $(XDG_DATA_HOME),$(HOME)/.local/share)/icons/hicolor/256x256/apps
# A BUILD FROM A DIRTY TREE HAS TO SAY SO IN EVERY STAMP IT CARRIES, and this
# repository is the case that makes it matter rather than a hypothetical: three
# seats share one working tree with no branches, so uncommitted work is the
# normal state and not the exception.
#
# `git describe --dirty` marked VERSION and `git rev-parse` cannot mark SHA, so
# one binary carried a true answer and a false one. `make version` would print a
# product string ending in -dirty beside a commit that was never built, and the
# commit is the field anybody actually quotes when a build misbehaves.
#
# UNTRACKED FILES COUNT HERE, WHICH `git describe --dirty` DOES NOT DO. Go
# compiles every .go file in a package directory whether git knows about it or
# not, so a whole new package arrives untracked, changes the binary, and leaves
# describe calling the tree clean. internal/coord arrived exactly that way.
# `git status --porcelain` honours .gitignore, so build/ does not trip it.
#
# One definition, applied to both stamps, because two definitions of dirty is
# the bug this is fixing.
DIRTY      := $(shell test -z "$$(git status --porcelain 2>/dev/null)" || printf -- -dirty)
VERSION    := $(shell git describe --tags --always 2>/dev/null || echo dev)$(DIRTY)
WIRE       := v1
SHA        := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)$(DIRTY)
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
              -X main.version=$(VERSION) -X main.wire=$(WIRE) \
              -X main.sha=$(SHA)         -X main.date=$(DATE)
GOFLAGS    := -trimpath
COVER_MIN  := 90
RATCHET    := size-ratchet.json
# The binaries `build` produces and the ratchet guards, in ONE list so
# bench-size, bench-size-update and bench-size-one cannot drift apart. The
# window is deliberately not here: it needs cgo and a webview, and ci must
# not assume either, so it keeps its own pair of targets.
RATCHET_BINS := $(BIND) $(BIN) fakeapp ledger abacus
# Only used to serve the window's built page to the contrast gate. Nothing
# listens on it outside that target.
CONTRAST_PORT ?= 8731
# Set explicitly rather than left to the runtime's defaults (section 22),
# and the same values the unit file will carry.
GOMEMLIMIT ?= 64MiB
GOGC       ?= 100
COVER_OUT  := build/cover.out
# Every package EXCEPT the window, and this is not a convenience.
#
# `cmd/rigwindow` is the only cgo binary: through Wails it needs gtk4 and
# webkitgtk-6.0 headers via pkg-config. `go vet ./...`, `go test ./...` and
# `golangci-lint run ./...` all LOAD every package they are given, so a
# module-wide scope needs those headers present even though nothing in the
# daemon or the CLI links them. `ubuntu-latest` has neither, so the whole gate
# went red on a machine this project deliberately supports - and it went red on
# the first push after the window landed rather than when the window was
# written, which is the worst time to find out.
#
# Section 17 already says the daemon side must build and test with no webview.
# This is that rule applied to the tool scopes, where it had never been
# applied. `make vet-window` and `make test-window` cover the window on a
# machine that has the deps.
GO_PKGS    := $(shell go list ./... | grep -v '/cmd/rigwindow')
# The same set as directories, because golangci-lint takes paths and not import
# paths: handed an import path it looks for a directory of that name under the
# working tree, fails to stat it, and reports "0 issues" beside a typechecking
# error and exit 7. A gate that exits non-zero while printing zero problems is
# worse than one that fails loudly, so the two forms are separate variables
# rather than one that is right for half its callers.
LINT_DIRS  := $(shell go list -f '{{.Dir}}' ./... | grep -v '/cmd/rigwindow')
CHAOS_ITER ?= 10000
FUZZ_TIME  ?= 60s

.DEFAULT_GOAL := help
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c

##@ Build

# Every binary bench-size measures must be built here, or `make ci` fails on a
# cold checkout while passing on a warm one. build-abacus was missing and
# bench-size measured build/abacus anyway, so ci depended on a binary it never
# built - invisible locally because the file was left over from an earlier run.
build: build-rigd build-rig build-fakeapp build-ledger build-abacus ## Build every binary into build/

build-rigd: ## Build the daemon (links none of the terminal stack)
	@mkdir -p build
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/$(BIND) ./cmd/rigd

build-rig: ## Build the client (links none of the daemon's internals)
	@mkdir -p build
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/$(BIN) ./cmd/rig

build-fakeapp: ## Build the reference program the conformance suite drives
	@mkdir -p build
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/fakeapp ./cmd/fakeapp

# M1a's first fake application (section 5h, section 23), modelled on
# pull-report's measured markup. It serves its own pane and the element kit
# beside it, because rig has no HTTP server to publish the kit from until M2.
#
# The copy is the whole reason this is a target rather than a plain go build:
# go:embed cannot reach above its own package, exactly as it cannot for the
# window's built frontend, so design/kit is the source and cmd/ledger/kit is
# where it has to sit to be embedded. rsync is not assumed; cp -R is.
build-ledger: ## Build M1a's first fake application, kit and all
	@mkdir -p build cmd/ledger/kit
	@find cmd/ledger/kit -mindepth 1 ! -name .gitkeep -delete
	cp design/kit/kit.css design/kit/kit.js design/kit/pane.js cmd/ledger/kit/
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/ledger ./cmd/ledger

build-abacus: ## Build M1a's second fake application, on dispatch's shape
	# Its own target rather than a flag on build-ledger, for the same reason it
	# is its own binary: section 5h says a fake application written to fit the
	# kit proves nothing, and a second adopter that shares the first one's
	# build is a second adopter in name only.
	@mkdir -p build cmd/abacus/kit
	@find cmd/abacus/kit -mindepth 1 ! -name .gitkeep -delete
	cp design/kit/kit.css design/kit/kit.js design/kit/pane.js cmd/abacus/kit/
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/abacus ./cmd/abacus

# The window is the third binary (section 17, section 22) and deliberately not
# part of `build`: it is the only one that needs cgo, gtk and a webview, so a
# machine without those can still build and test everything else. That is also
# why ci does not call it. It carries its own ratchet row for the same reason
# the split exists - it is twice the size of the CLI.
# Split from build-rigwindow so the frontend can be built, and measured, on a
# machine with no webview: contrast-window needs the page and not the binary.
deps-frontend: ## Install the frontend's pinned dependencies from the lockfile
	# `npm ci` and not `npm install`, because ci installs exactly what
	# package-lock.json pins and fails if the lockfile and package.json
	# disagree, where install would quietly resolve a newer tree and rewrite
	# the lock. A gate measuring a build nobody can reproduce is not a gate.
	#
	# Its own target so the workflow can call it: section 28 says CI defines
	# nothing that is not a Makefile target, and `npm ci` written as a raw
	# workflow step is a second definition of how this repo installs.
	cd frontend && npm ci

build-frontend: ## Build the window's frontend into cmd/rigwindow/dist
	@find cmd/rigwindow/dist -mindepth 1 ! -name .gitkeep -delete
	cd frontend && npm run build

build-rigwindow: build-frontend ## Build the window (needs cgo, gtk3 and webkit2gtk)
	# design/tray is the source and cmd/rigwindow/icons is where it has to sit
	# to be embedded (section 11), same reason and same shape as build-ledger's
	# kit copy above: go:embed cannot reach above its own package.
	@mkdir -p build cmd/rigwindow/icons
	@find cmd/rigwindow/icons -mindepth 1 ! -name .gitkeep -delete
	# The -down pair is the badged variant section 11 rules for a daemon that
	# is not answering. Generated by design/tray/make-down-icons.py and
	# committed beside its sources, so this copies four files and not two.
	cp design/tray/development.png design/tray/production.png \
	   design/tray/development-down.png design/tray/production-down.png \
	   cmd/rigwindow/icons/
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o build/rigwindow ./cmd/rigwindow

build-all: ## Cross-compile for every supported target
	@mkdir -p build
	@for t in linux/amd64 linux/arm64; do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "  $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
	    -o build/$(BIN)-$$os-$$arch ./cmd/rig; \
	done

install-selftest: build ## Prove install's ROLLBACK catches, on a throwaway unit
	@echo
	@echo "  The rollback is the branch a successful install never runs, and it"
	@echo "  guards the one outcome section 28 forbids: a machine with no"
	@echo "  working daemon. This drives it on a throwaway unit with its own"
	@echo "  prefix, runtime dir and unit name. rigd.service, rigwindow.service"
	@echo "  and rig-team.service are never named and never signalled."
	@echo
	@tools/install-selftest.sh

install: build ## Install rig and rigd, replacing a live deployment without losing anything
	@echo
	@echo "  Replacing the deployment, if there is one. PLAN.md section 28."
	@echo "  A stale daemon left running IS a failed termination - it is just"
	@echo "  the one nobody looks at, because nothing crashed."
	@echo
	@tools/install-rigd.sh "$(PREFIX)" "$(UNITDIR)" build "$(VERSION)"
	@echo
	@echo "installed $(PREFIX)/bin/$(BIN) and $(PREFIX)/bin/$(BIND) ($(VERSION))"
	@echo
	@echo "ENABLING IS STILL NOT THIS TARGET'S CALL. Replacing what is already"
	@echo "deployed and deciding to deploy are different acts, and only the"
	@echo "first one is done here:"
	@echo "    systemctl --user enable --now $(BIND).service"
	@echo
	@echo "THE WINDOW AND TRAY ARE NOT INSTALLED BY THIS TARGET: make install-window"
	@echo "  It is deliberately separate: the tray is on a screen somebody is"
	@echo "  looking at, and rigwindow is Wants=rigd.service rather than"
	@echo "  Requires=, so it rides out the daemon restart above by design."

# install-window installs the window and its tray, and it is SEPARATE FROM
# `install` on purpose.
#
# ⛔ NOT FOLDED IN, for the same reason `ci` excludes cmd/rigwindow: it is the
# only cgo binary and needs gtk3 and webkit2gtk, which `install` must not
# assume a machine has. A single target would make installing the daemon fail
# on a box that only ever wanted the daemon.
#
# WHY IT EXISTS AT ALL is an acceptance test rather than a convenience. Boris,
# 2026-09-16: "I'll know we reached MVP when I'll see the production icon on my
# system-tray both during this session and after I reboot the machine so I know
# it is properly deployed." Nothing installed the window before this target and
# nothing started it after a login, so the second half of that sentence could
# not have passed however many times the first half did.
install-window: build-rigwindow ## Install the window, tray, user service and desktop launcher
	install -Dm755 build/rigwindow $(PREFIX)/bin/rigwindow
	install -Dm644 packaging/rigwindow.service $(UNITDIR)/rigwindow.service
	# The launcher and its icon. Installed HERE and not by hand, because a
	# launcher that exists only in somebody's home directory is one reinstall
	# from gone with no trace in any repository - which is exactly how this one
	# was found on 2026-09-17, twenty minutes after it was made.
	install -Dm644 packaging/rig.desktop $(APPDIR)/rig.desktop
	install -Dm644 cmd/rigwindow/icons/production.png $(ICONDIR)/rig.png
	@systemctl --user daemon-reload 2>/dev/null || \
	  echo "no user systemd here; the unit is installed but not registered"
	@update-desktop-database $(APPDIR) 2>/dev/null || true
	@echo "installed $(PREFIX)/bin/rigwindow ($(VERSION))"
	@echo "installed $(UNITDIR)/rigwindow.service"
	@echo "installed $(APPDIR)/rig.desktop and $(ICONDIR)/rig.png"
	@echo
	@echo "NOT ENABLED, the same call as the daemon's and for the same reason."
	@echo "The tray shows which estate rigd is, so BOTH are needed for an icon:"
	@echo "    systemctl --user enable --now $(BIND).service"
	@echo "    systemctl --user enable --now rigwindow.service"

# redeploy is the ONE CALL, and it exists because several was the interface he
# had to type. PLAN.md section 28, Boris 2026-09-18: "We should have a Make
# target that builds and re-deploys everything; It's easier that doing multiple
# Make calls" and "Re-deployment should elegantly gracefully close the live
# instances and deploy the new ones instead of them."
#
# ⛔ A COMPOSITE, NOT A REPLACEMENT. `install` and `install-window` keep their
# contracts and their separate existence: `install` must not need gtk3 and
# webkit2gtk, because a box that only ever wanted the daemon must still be able
# to install one. This target is for the machine that wants both.
#
# ⛔ THE ORDER IS THE REQUIREMENT. Both binaries are built AND both answer
# `--version` before anything live is stopped - clause 1 applied to two halves
# at once, so a window that will not build cannot take the daemon down with it.
#
# ⛔ WHAT IT DELIBERATELY DOES NOT TOUCH, and each one is a separate
# deployment: `rig-team.service` on its own XDG_STATE_HOME, and any private
# daemon a peer is running on its own XDG_RUNTIME_DIR. Clause 7 says stopping
# "the daemon" by pattern would take a peer's work with it, and that rule is
# not narrowed by wanting one call.
redeploy: build build-rigwindow ## Build and redeploy EVERYTHING live - daemon, client and window
	@build/$(BIND) --version >/dev/null 2>&1 || { \
	  echo "the new $(BIND) does not answer --version; NOTHING was replaced"; exit 1; }
	@build/rigwindow --version >/dev/null 2>&1 || { \
	  echo "the new rigwindow does not answer --version; NOTHING was replaced"; exit 1; }
	@echo
	@echo "  Both new binaries answer --version. Replacing what is live."
	@echo
	@$(MAKE) --no-print-directory install
	@$(MAKE) --no-print-directory install-window
	@echo
	@echo "  The window's turn, and this is the half a plain install-window"
	@echo "  never did: the file was replaced and the RUNNING tray kept its"
	@echo "  own deleted inode. PLAN.md section 28."
	@echo
	@tools/restart-window.sh rigwindow.service
	@echo
	@echo "redeployed to $(VERSION):"
	@echo "    $(PREFIX)/bin/$(BIN), $(PREFIX)/bin/$(BIND), $(PREFIX)/bin/rigwindow"
	@echo
	@echo "NOT TOUCHED, on purpose - each is a separate deployment:"
	@echo "    rig-team.service, and any daemon on a private XDG_RUNTIME_DIR"

uninstall: ## Remove the installed binaries and the user service
	-@systemctl --user disable --now $(BIND).service 2>/dev/null || true
	rm -f $(PREFIX)/bin/$(BIN) $(PREFIX)/bin/$(BIND)
	rm -f $(UNITDIR)/$(BIND).service
	rm -f $(APPDIR)/rig.desktop $(ICONDIR)/rig.png
	@systemctl --user daemon-reload 2>/dev/null || true

run: build-rigd ## Run the daemon in the foreground with debug logging
	GOMEMLIMIT=$(GOMEMLIMIT) GOGC=$(GOGC) ./build/$(BIND) --log-level=debug

dev: ## Run the daemon with reload on save (needs air)
	@command -v air >/dev/null || { echo "air not found: go install github.com/air-verse/air@latest"; exit 1; }
	air -c .air.toml

clean: ## Remove build output, coverage and generated artefacts
	rm -rf build dist
	# The window's assets are embedded, so a clean that leaves them behind is
	# how yesterday's frontend ships inside today's binary. .gitkeep stays:
	# go:embed needs the directory to exist even when nothing is built.
	@test -d cmd/rigwindow/dist && find cmd/rigwindow/dist -mindepth 1 ! -name .gitkeep -delete || true

##@ Test

test: test-unit test-race ## Run the standard test suite (unit + race)

test-unit: ## Run unit tests
	go test $(GO_PKGS)

test-race: ## Run tests under the race detector
	go test -race $(GO_PKGS)

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

# gate is the scoped answer to "is what I touched clean", which neither `ci`
# nor `lint` can give on a tree several seats share. BACKLOG.md B84.
gate: ## Gate ONLY the packages you touched (or named), and say what it did not cover
	@go run ./cmd/gate $(GATE_ARGS)

lint: lint-house ## Run golangci-lint plus the house analyzers
	golangci-lint run $(LINT_DIRS)

# The house rules that no off-the-shelf linter knows. Separate from lint
# because these need nothing installed - they are go run over this module -
# so ci can carry them while golangci-lint is still being configured.
#
# Each walks the filesystem rather than loading packages, so one run covers
# every build tag rather than the default one (section 5i wants them run
# "under every tag set CI builds"). internal/analysis has the reasoning.
lint-house: ## Run the four house analyzers (sections 3, 14, 21, 29)
	go run ./internal/analysis/cmd/noprogramid ./...
	go run ./internal/analysis/cmd/noregistryhandle ./...
	go run ./internal/analysis/cmd/noenumzero ./...
	go run ./internal/analysis/cmd/nocontextfree ./...

fmt: ## Format Go and frontend sources
	gofmt -s -w .
	go run golang.org/x/tools/cmd/goimports@latest -w .
	cd frontend && npm run format

vet: ## go vet
	go vet $(GO_PKGS)

vet-window: ## go vet the window (needs gtk4 and webkitgtk-6.0)
	go vet ./cmd/rigwindow/...

test-window: ## Test the window (needs gtk4 and webkitgtk-6.0)
	go test ./cmd/rigwindow/...

audit: ## Check dependencies for known vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

verify: ## NOT YET (M1, with slice 8): `rig verify` does not exist
	@echo 'make verify: not implemented. `rig verify` is the conformance'
	@echo '  suite, and it belongs with M1 slice 8 rather than with a'
	@echo '  verb-filling exercise - `make ci` help already lists it as'
	@echo '  waiting on M1. The body below is what it should run once the'
	@echo '  verb exists: fakeapp clean, then each --misbehave must FAIL.'
	@echo '  Misbehaviours: hang crash leak flood lie ignore-cancel garbage'
	@exit 1

contrast: contrast-selftest ## Measure WCAG contrast in a real browser, both themes
	# Five passes, because no single instrument sees all five things: text nodes
	# against their composited ground, SVG <text> against the rects behind it,
	# boundaries and focus indicators at 1.4.11's 3:1 (which no text pass can
	# see at all - they walk nodeType 3, and neither a border nor a ring is a
	# text node), and painted colour for anything dimmed or color-mixed. The
	# target used to invoke this file with --themes and --fail-under; the file
	# had never existed, so the target exited "Cannot find module" and no
	# workflow called it. While it was down, the focus ring shipped at 2.17:1
	# dark and 1.48:1 light against a 3:1 requirement.
	node tools/contrast-audit.mjs design/visual-system.html

contrast-window: contrast-selftest build-frontend ## Measure the shell's own page, both themes
	# The shell is a second page the gate has to see, and it cannot be audited
	# from disk: Chrome refuses a module script over file://, so the page loads,
	# renders nothing, and every pass measures zero. That combination used to
	# print "clean"; the auditor now calls a zero measurement a failure, and
	# this target serves the page so there is something to measure.
	#
	# ?fixture=1 puts programs in the rail. Without it the rail is empty, and
	# the rail's focus ring is the single thing section 20's gate was rebuilt
	# for - it shipped at 2.17:1 dark while the gate was down.
	#
	# ?pane=1 is a SECOND page and not a variant of the first, because one page
	# cannot show both: the pane draws whatever is selected, so a page showing a
	# program's own pane is a page not showing the detail list. It exists to gate
	# the unserved state, whose colours nothing measured until now.
	#
	# The record views (?gui=projects, ?gui=spec, ?gui=decisions) left with
	# the planner on 2026-09-24 (plan/50 move 7); their colours are measured
	# by docket's own pane gate. Auditing them here measured a shell showing
	# nothing, which reads green and proves nothing.
	#
	# What it does NOT gate is the frame's focus ring, and that is a finding
	# rather than a gap - see the note in frontend/src/lib/Pane.svelte. The ring
	# lands on the program's own document, which rig does not own, so there is no
	# ground to measure it against that means anything.
	@python3 -m http.server $(CONTRAST_PORT) --directory cmd/rigwindow/dist >/dev/null 2>&1 & \
	  srv=$$!; \
	  trap 'kill $$srv 2>/dev/null || true' EXIT; \
	  sleep 1; \
	  node tools/contrast-audit.mjs \
	    'http://127.0.0.1:$(CONTRAST_PORT)/index.html?fixture=1' \
	    'http://127.0.0.1:$(CONTRAST_PORT)/index.html?pane=1' \
	    'http://127.0.0.1:$(CONTRAST_PORT)/index.html?dash=1' \
	    'http://127.0.0.1:$(CONTRAST_PORT)/index.html?settings=1'

theme-gate: ## Prove the palette engine refuses an unreadable token set
	# Section 6 promises rig refuses a token set that cannot be read and names
	# the token and the ground. checkTheme in design/theme.js is that promise,
	# and the settings panel and this target are the same call - so the window,
	# the design page and the gate cannot disagree about what unreadable means.
	#
	# --selftest carries seven known answers, five of them deliberately
	# unreadable themes, and fails if any is called clean. Same reasoning as
	# contrast-selftest: this gate's whole job is to say no, so one that cannot
	# say no is worse than none.
	#
	# It shipped at 9db5a4b with no target and therefore no CI step. A gate that
	# exists and is not called is the state that let the focus ring ship at
	# 2.17:1 - see the comment on `contrast`. That is why this is in `ci`.
	node tools/theme-gate.mjs --selftest

contrast-selftest: ## Prove the contrast instruments against known answers first
	# A script that lies is worse than no script: the SVG audit shipped for
	# months dropping alpha, and on its own self-test found 0 of 1 real failures
	# while inventing 2. `contrast` depends on this, so the gate cannot report
	# on a page through a broken instrument.
	node tools/contrast/selftest/run.mjs

##@ Generate

# `docs` is NOT a step of generate: it names a `rig docs` verb that has never
# existed, and as a step it made `make generate` fail at its last target.
generate: proto schema types ## Regenerate everything that is generated

proto: ## Generate Go from proto/
	# protoc directly, not buf: buf earns its keep on a multi-module workspace
	# with remote dependencies and there is one file here with none. The -I .
	# and the full path are load-bearing - the source path is embedded in the
	# descriptor, so generating it any other way rewrites the whole file.
	protoc --go_out=. --go_opt=module=$(MODULE) -I . proto/rig/v1/wire.proto
	gofmt -s -w proto/

schema: ## Emit the declaration JSON Schema from the proto descriptors
	go run ./cmd/schemagen -out schema/

schema-check: ## Fail if the committed schema is not what schemagen emits
	@tmp=$$(mktemp -d); \
	 go run ./cmd/schemagen -out $$tmp >/dev/null; \
	 if diff -ru schema/ $$tmp/ >/dev/null 2>&1; then \
	   rm -rf $$tmp; \
	 else \
	   diff -ru schema/ $$tmp/ || true; \
	   rm -rf $$tmp; \
	   echo; \
	   echo "  schema/ is not what cmd/schemagen emits. Run: make schema"; \
	   exit 1; \
	 fi

types: schema ## Generate TypeScript types from the JSON Schemas
	cd frontend && npm run gen:types

# plan/28 lists `docs` in the Generate group, so the target stays and says it
# is not built rather than disappearing. What it named, `rig docs --out
# docs/cli.md`, never existed; the capability map it also named is served live
# as the MCP `capabilities` resource.
docs: ## NOT YET: `rig docs` does not exist (plan/28 lists the target)
	@echo 'make docs: not implemented. It named `rig docs --out docs/cli.md`, a'
	@echo '  verb no commit has ever added. plan/28 lists the target and no'
	@echo '  section specifies what the reference contains. The capability map'
	@echo '  is served live as the MCP `capabilities` resource.'
	@exit 1

##@ Measure

bench: ## Run Go benchmarks
	go test -bench=. -benchmem -run=NONE ./...

bench-ipc: ## Reproduce the transport numbers in PLAN.md section 4
	@mkdir -p build
	go build $(GOFLAGS) -o build/ipcbench ./cmd/ipcbench
	./build/ipcbench

bench-size: build ## Record or check the binary-size ratchet (PLAN.md 17, 22)
	go run ./cmd/sizeratchet --ratchet $(RATCHET) \
	  $(addprefix --bin build/,$(RATCHET_BINS))

bench-size-update: build ## Accept the current sizes as the new ratchet
	go run ./cmd/sizeratchet --ratchet $(RATCHET) --update \
	  $(addprefix --bin build/,$(RATCHET_BINS))

# One binary's row, without building anybody else's. bench-size depends on
# `build`, so the only way to reach one row was to build all five - which means
# a seat could never gate the row its own commits move. On 2026-09-11 the seat
# that grew rigd by a page could not check it for exactly that reason, and CI
# caught it on main instead. That is the whole case for this target.
#
# B, because BIN and BIND are taken above. An explicit target rather than a
# bench-size-% pattern rule, because `make help` reads `^[a-zA-Z_-]+:.*##` and
# cannot see a % - a target nobody can find is a target nobody runs.
#
# The name is checked against RATCHET_BINS rather than passed straight through:
# sizeratchet WRITES a row for a binary it has never seen, on a plain run and
# by design, so an unguarded B would silently add a row to a shared file.
bench-size-one: ## Check ONE binary's ratchet row: make bench-size-one B=rigd
	@test -n "$(B)" || { echo "usage: make bench-size-one B=<one of: $(RATCHET_BINS)>"; exit 1; }
	@case " $(RATCHET_BINS) " in *" $(B) "*) ;; \
	  *) echo "bench-size-one: no ratchet row for '$(B)'; one of: $(RATCHET_BINS)"; exit 1 ;; \
	esac
	$(MAKE) --no-print-directory build-$(B)
	go run ./cmd/sizeratchet --ratchet $(RATCHET) --bin build/$(B)

# ledger is NOT split out the way the window is. The window's split exists for
# one reason - it needs cgo, gtk and a webview - and none of that applies to a
# pure-Go fixture, so ledger builds in `build` and is measured by `bench-size`,
# which means CI guards it on every push. An unguarded binary is how the window
# sat 1.74 MB over an unread row for four commits.
#
# The window's row is checked separately because building it needs a webview.
# sizeratchet merges rather than replaces, so these two invocations share one
# file without either one dropping the other's rows.
bench-size-window: build-rigwindow ## Check the window against its ratchet row
	go run ./cmd/sizeratchet --ratchet $(RATCHET) --bin build/rigwindow

bench-size-window-update: build-rigwindow ## Accept the window's current size
	go run ./cmd/sizeratchet --ratchet $(RATCHET) --update --bin build/rigwindow

bench-idle: build ## Measure idle footprint against the budget in PLAN.md section 17
	go run ./cmd/footprint --binary build/$(BIND) --quiet-for 60s \
	  --max-rss 20MiB --max-cpu 0.1 --max-wakeups 1

bench-scale: build ## Measure the per-program overhead with 1, 10 and 50 programs registered
	go run ./cmd/footprint --binary build/$(BIND) --programs 1,10,50 --max-delta 500KiB

build-minimal: ## Build the kernel-only daemon, no services, no surfaces
	@mkdir -p build
	go build $(GOFLAGS) -tags minimal -ldflags '$(LDFLAGS)' -o build/$(BIND)-minimal ./cmd/rigd
	@ls -la build/$(BIND)-minimal

modules: ## List every service, surface and renderer, and prove none imports another
	go run ./cmd/rig modules list
	go run ./internal/analysis/cmd/nomodulecross ./...

modules-matrix: ## Build kernel-plus-one for every module in turn (PLAN.md 5i)
	@set -e; mods="$$(cat modules.txt 2>/dev/null || true)"; \
	 echo "  N=0 (build-minimal)"; $(MAKE) --no-print-directory build-minimal; \
	 if [ -z "$$mods" ]; then \
	   echo "  no modules yet: the matrix is the N=0 case alone."; \
	   echo "  Each module adds a line to modules.txt as it lands (5i)."; \
	 else \
	   for m in $$mods; do \
	     echo "  kernel + $$m"; \
	     go build $(GOFLAGS) -tags "minimal $$m" -o /dev/null ./cmd/rigd; \
	   done; \
	 fi

profile: ## NOT YET: dead twice over - no `rig serve`, and no --pprof anywhere
	@echo 'make profile: not implemented, and it is dead in TWO ways rather'
	@echo '  than one. `rig serve` does not exist - and unlike the other dead'
	@echo '  verbs it is not waiting on a milestone, because nothing in the'
	@echo '  plan specifies it: serving is rigd, and a client cannot serve.'
	@echo '  Second, `rigd --help` has only -log-level and -version, so the'
	@echo '  --pprof flag this named exists nowhere either. Exposing pprof is'
	@echo '  a decision about a debug surface on the daemon, not a fix here.'
	@exit 1

##@ Operate

# Five of the targets below name a `rig` verb that does not exist yet, and
# they used to advertise it as working. `rig` treats an unknown verb as a
# PROGRAM NAME, so `make doctor` answered `no program "doctor" is connected` -
# a message about the estate, for a command nobody has written. Each now says
# which milestone it waits on and refuses, rather than failing as if the
# estate were at fault. The rule for adding one: a target may name an
# unwritten verb, but it must not claim the verb works.

up: ## NOT YET (M6, control and supervision): `rig up` does not exist
	@echo 'make up: not implemented. `rig up` is M6 (start, stop, restart,'
	@echo '  health). It could not work from cold in any case: rig is a'
	@echo '  client, so with no daemon there is nothing to dial. Start the'
	@echo '  daemon directly:  XDG_RUNTIME_DIR=... rigd'
	@exit 1

down: build ## Stop the daemon serving this XDG_RUNTIME_DIR
	./build/$(BIN) down

doctor: ## NOT YET (M5, observability): `rig doctor` does not exist
	@echo 'make doctor: not implemented. `rig doctor` ships at M5 with its'
	@echo '  dependency check (PLAN.md section 23, M5 row). Section 5 makes it a'
	@echo '  GRANTED surface - an estate-wide aggregate readable with'
	@echo '  introspect - so it is an authorisation decision, not a stub.'
	@exit 1

apps: build ## List registered programs and their state
	./build/$(BIN) apps list

logs: ## NOT YET (M5, observability): `rig logs` does not exist
	@echo 'make logs: not implemented. `rig logs` ships at M5 with log, trace'
	@echo '  and metric ingest and the call log (PLAN.md section 23, M5 row).'
	@exit 1

tui: ## NOT YET (M3, terminal client): `rig tui` does not exist
	@echo 'make tui: not implemented. The `rig tui` frame ships at M3 with'
	@echo '  `rig shell` and huh forms (PLAN.md section 23, M3 row).'
	@exit 1

##@ Ship

tidy: ## Tidy go.mod and npm dependencies
	go mod tidy
	cd frontend && npm prune

deps-check: ## Check the build against the stack table, plan section 22 (NOT upstream drift: that needs the network)
# PLAN.md became an index on 2026-09-12 and the sections live in plan/. depscheck
# needed no code change: it finds the "## 22. Tech stack" heading and reads to the
# next "## " or to EOF, which is what that section's own file gives it.
	go run ./cmd/depscheck --plan plan/22-tech-stack.md

release: ci ## Tag, generate the changelog and build release artefacts
	@test -n "$(V)" || { echo "usage: make release V=1.2.3"; exit 1; }
	git tag -a v$(V) -m "release v$(V)"
	git cliff --tag v$(V) -o CHANGELOG.md
	git add CHANGELOG.md && git commit -m "chore(release): v$(V)"
	$(MAKE) build-all package

package: build ## Build the .deb from freshly built binaries
	@mkdir -p dist
	go run ./cmd/pkgdeb --version $(VERSION) --out dist/

ci: fmt-check vet lint-house test-race schema-check deps-check theme-gate ## Everything CI runs
	@echo
	@echo "  M0's gate. Targets not yet in ci, each waiting on the milestone"
	@echo "  that gives it something to check:"
	@echo "    lint       golangci-lint on top       green, and CI runs it as"
	@echo "               its own step - it needs the binary installed, which"
	@echo "               ci must not assume a machine has"
	@echo "    cover      90% on internal/           M1, once there is a registry"
	@echo "    modules    the layering analyzer      M1"
	@echo "    test-wire  golden wire vs last tag    M1, needs a tagged release"
	@echo "    verify     conformance vs fakeapp     M1"
	@echo "    bench-idle idle footprint             runs today, and stays out:"
	@echo "               and bench-scale            64s against this gate's 3s,"
	@echo "                                          60 of which is the observation"
	@echo "                                          window and not shortenable"
	@echo "                                          without making the number"
	@echo "                                          meaningless. Wakeups and CPU"
	@echo "                                          are also noisy on a loaded or"
	@echo "                                          throttled machine. Schedule it"
	@echo "    audit      govulncheck                any time, needs network"
	@echo "    bench-size the binary-size ratchet     REMOVED FROM ci 2026-09-12"
	@echo "               by Boris. It compared strictly-greater against the"
	@echo "               previous build, so it had no headroom and fired on"
	@echo "               any growth at all - every firing was answered by"
	@echo "               re-recording the number. The target still works and"
	@echo "               is now run deliberately. PLAN.md section 17"
	@echo "    contrast   WCAG in a real browser      green, and CI runs it as"
	@echo "               its own step - it needs a browser, which ci must not"
	@echo "               assume a machine has"

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

.PHONY: build build-rigd build-rig build-fakeapp build-ledger build-abacus deps-frontend build-frontend build-rigwindow build-all install uninstall \
        run dev clean test test-unit test-race \
        test-chaos test-e2e test-wire fuzz cover cover-html lint lint-house fmt vet audit \
        vet-window test-window verify contrast contrast-selftest contrast-window theme-gate generate proto schema types docs bench bench-ipc profile \
        up down doctor apps logs tui tidy deps-check release package ci fmt-check \
        bench-idle bench-scale bench-size bench-size-update bench-size-one build-minimal \
        bench-size-window bench-size-window-update \
        modules modules-matrix version help
