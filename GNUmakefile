# kpm — build, test, release (GNU Make). On FreeBSD use gmake (pkg install gmake).
# GNU Make prefers GNUmakefile over Makefile; the Makefile stub forwards to gmake.

BINARY   := kpm
PLUGIN   := kubectl-kpm
DIST     := dist
PREFIX   ?= /usr/local
BINDIR   ?= $(PREFIX)/bin
MANDIR   ?= $(PREFIX)/share/man
FREEBSD_ARCH ?= amd64
OPENBSD_ARCH ?= amd64
# Minimum total statement coverage for `make cover-check`.
COVERAGE_MIN ?= 80
GOBIN    ?= $(shell go env GOPATH)/bin
VERSION  ?= $(shell v=$$(cat VERSION 2>/dev/null | tr -d '\n\r'); [ -n "$$v" ] && echo "v$$v" || echo "v0.0.0")
COMMIT   := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BRANCH   := $(shell b=$$(git rev-parse --abbrev-ref HEAD 2>/dev/null); [ -n "$$b" ] && [ "$$b" != "HEAD" ] && echo "$$b" || echo "unknown")
BUILDDATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -ldflags "-s -w -X github.com/hrodrig/kpm/internal/cli.Version=$(VERSION) -X github.com/hrodrig/kpm/internal/cli.Commit=$(COMMIT) -X github.com/hrodrig/kpm/internal/cli.BuildDate=$(BUILDDATE) -X github.com/hrodrig/kpm/internal/cli.Branch=$(BRANCH)"
PORT_VERSION := $(shell cat VERSION 2>/dev/null | tr -d '\n\r' | sed 's/^v//')

GREEN  := \033[0;32m
YELLOW := \033[0;33m
CYAN   := \033[0;36m
RESET  := \033[0m
ifneq ($(NO_COLOR),)
  GREEN  :=
  YELLOW :=
  CYAN   :=
  RESET  :=
endif

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo "$(GREEN)kpm$(RESET) — Kubernetes Pods Metrics"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "$(YELLOW)Build:$(RESET)"
	@echo "  $(GREEN)build$(RESET)                     Build ./bin/kpm for current platform"
	@echo "  $(GREEN)build-all$(RESET)                 Cross-compile to $(DIST)/"
	@echo ""
	@echo "$(YELLOW)Install & clean:$(RESET)"
	@echo "  $(GREEN)install$(RESET)                   go install to \$$GOBIN"
	@echo "  $(GREEN)install-man$(RESET)               Install man pages to \$$(MANDIR)"
	@echo "  $(GREEN)install-kubectl-plugin$(RESET)    Install kubectl-kpm shim (\$$(BINDIR))"
	@echo "  $(GREEN)clean$(RESET)                     Remove ./bin/kpm, coverage.out, and $(DIST)/"
	@echo ""
	@echo "$(YELLOW)Test:$(RESET)"
	@echo "  $(GREEN)test$(RESET)                      Unit tests (go test -race ./...)"
	@echo "  $(GREEN)cover$(RESET)                     Unit tests with coverage.out"
	@echo "  $(GREEN)cover-check$(RESET)               Fail if total statement coverage < $(COVERAGE_MIN)%"
	@echo ""
	@echo "$(YELLOW)Quality:$(RESET)"
	@echo "  $(GREEN)lint$(RESET)                      check-pins, check-man-version, gofmt -s, go vet, gocyclo (<=14)"
	@echo "  $(GREEN)lint-fix$(RESET)                  gofmt -s -w"
	@echo "  $(GREEN)security$(RESET)                  govulncheck + gocyclo + grype"
	@echo "  $(GREEN)tools$(RESET)                     Install govulncheck, gocyclo and grype"
	@echo ""
	@echo "$(YELLOW)Docker:$(RESET)"
	@echo "  $(GREEN)docker-build$(RESET)              Build the image locally (kpm:$(VERSION))"
	@echo "  $(GREEN)docker-scan$(RESET)               Build and grype-scan the image"
	@echo ""
	@echo "$(YELLOW)Release:$(RESET)"
	@echo "  $(GREEN)release-check$(RESET)             VERSION semver + lint + test + cover-check + security + docker-scan"
	@echo "  $(GREEN)release$(RESET)                   release-check then goreleaser (only from main)"
	@echo "  $(GREEN)snapshot$(RESET)                  Goreleaser snapshot to $(DIST)/ (no tag)"
	@echo "  $(GREEN)port-freebsd-sync$(RESET)         Set PORTVERSION in contrib/freebsd/Makefile from VERSION"
	@echo "  $(GREEN)port-openbsd-sync$(RESET)         Sync OpenBSD port Makefile to VERSION"
	@echo "  $(GREEN)dist-freebsd$(RESET)               FreeBSD release tarball under $(DIST)/"
	@echo "  $(GREEN)dist-openbsd$(RESET)               OpenBSD release tarball under $(DIST)/"
	@echo ""
	@echo "$(CYAN)Current version:$(RESET) $$(cat VERSION 2>/dev/null | tr -d '\n\r' || echo '?') (ldflags $(VERSION), branch $(BRANCH))"

.PHONY: build build-all install clean docker-build docker-scan install-man install-kubectl-plugin
.PHONY: test cover cover-check lint lint-fix tools security release-check release snapshot
.PHONY: check-pins check-man-version check-docker govulncheck gocyclo grype
.PHONY: dist-freebsd dist-openbsd port-freebsd-sync port-openbsd-sync

build:
	@mkdir -p bin
	go build -trimpath $(LDFLAGS) -o bin/$(BINARY) ./cmd/kpm

build-all:
	@mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-amd64 ./cmd/kpm
	GOOS=linux GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-linux-arm64 ./cmd/kpm
	GOOS=darwin GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-amd64 ./cmd/kpm
	GOOS=darwin GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-darwin-arm64 ./cmd/kpm
	GOOS=windows GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-windows-amd64.exe ./cmd/kpm
	GOOS=freebsd GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-freebsd-amd64 ./cmd/kpm
	GOOS=freebsd GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-freebsd-arm64 ./cmd/kpm
	GOOS=openbsd GOARCH=amd64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-openbsd-amd64 ./cmd/kpm
	GOOS=openbsd GOARCH=arm64 go build -trimpath $(LDFLAGS) -o $(DIST)/$(BINARY)-openbsd-arm64 ./cmd/kpm

install:
	go install -trimpath $(LDFLAGS) ./cmd/kpm

clean:
	rm -f bin/$(BINARY) coverage.out
	rm -rf $(DIST)

check-docker:
	@command -v docker >/dev/null 2>&1 || { echo "Error: docker not found — install Docker (or skip the Docker gates)."; exit 1; }
	@docker info >/dev/null 2>&1 || { echo "Error: the Docker daemon is not reachable — start Docker."; exit 1; }

docker-build: check-docker
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	  --build-arg BUILDDATE=$(BUILDDATE) --build-arg BRANCH=$(BRANCH) \
	  -t kpm:$(VERSION) .

docker-scan: check-docker
	@docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
	  --build-arg BUILDDATE=$(BUILDDATE) --build-arg BRANCH=$(BRANCH) \
	  -t kpm:scan .
	@if ! command -v grype >/dev/null 2>&1; then \
	  echo "grype not found — installing into $(GOBIN)..."; \
	  curl -sSfL https://get.anchore.io/grype | sh -s -- -b "$(GOBIN)"; \
	fi
	@PATH="$(GOBIN):$$PATH" grype kpm:scan -c .grype.yaml --fail-on high

install-man:
	@mkdir -p $(DESTDIR)$(MANDIR)/man1
	install -m644 contrib/man/man1/kpm.1 $(DESTDIR)$(MANDIR)/man1/kpm.1
	install -m644 contrib/man/man1/kubectl-kpm.1 $(DESTDIR)$(MANDIR)/man1/kubectl-kpm.1

install-kubectl-plugin:
	@mkdir -p $(DESTDIR)$(BINDIR)
	go build -trimpath $(LDFLAGS) -o $(DESTDIR)$(BINDIR)/$(PLUGIN) ./cmd/kpm

test:
	go test -race ./...

cover:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	@go tool cover -func=coverage.out | tail -1

cover-check:
	go test ./internal/... -coverprofile=coverage.out -covermode=atomic
	@pct=$$(go tool cover -func=coverage.out | grep '^total:' | awk '{print $$NF}' | tr -d '%'); \
	echo "Total statement coverage (internal/): $$pct% (minimum $(COVERAGE_MIN)% )"; \
	awk -v p="$$pct" -v m="$(COVERAGE_MIN)" 'BEGIN { if (p+0 < m+0) { print "Error: coverage is below " m "% — add tests or set COVERAGE_MIN="; exit 1 } }'

check-pins:
	@sh contrib/scripts/check-dependency-pins.sh

check-man-version:
	@expected="kpm v$$(tr -d '\n\r' < VERSION)"; \
	status=0; \
	for page in contrib/man/man1/kpm.1 contrib/man/man1/kubectl-kpm.1; do \
		got=$$(grep -m1 '^\.TH' "$$page" | sed -n 's/.*"\(kpm v[^"]*\)".*/\1/p'); \
		if [ "$$got" != "$$expected" ]; then \
			echo "check-man-version: FAIL ($$page .TH says '$$got', VERSION is '$$expected')"; \
			status=1; \
		else \
			echo "check-man-version: PASS ($$page $$got)"; \
		fi; \
	done; \
	exit $$status

gocyclo:
	@echo "Running gocyclo (complexity <= 14)..."
	@go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	@"$(shell go env GOPATH)/bin/gocyclo" -over 14 .

grype:
	@if ! command -v grype >/dev/null 2>&1; then \
	  echo "grype not found — installing into $(GOBIN)..."; \
	  curl -sSfL https://get.anchore.io/grype | sh -s -- -b "$(GOBIN)"; \
	fi
	@echo "Running grype (working tree, --fail-on high)..."
	@PATH="$(GOBIN):$$PATH" grype . -c .grype.yaml --fail-on high

lint: check-pins check-man-version
	@echo "Checking gofmt -s..."
	@unformatted=$$(gofmt -s -l .); [ -z "$$unformatted" ] || { echo "Files not formatted (run: make lint-fix):"; echo "$$unformatted"; exit 1; }
	@echo "Running go vet..."
	@go vet ./...
	@$(MAKE) --no-print-directory gocyclo

lint-fix:
	gofmt -s -w .

tools:
	go install golang.org/x/vuln/cmd/govulncheck@latest
	go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
	@if ! command -v grype >/dev/null 2>&1; then \
	  echo "Installing grype into $(GOBIN)..."; \
	  curl -sSfL https://get.anchore.io/grype | sh -s -- -b "$(GOBIN)"; \
	fi

govulncheck:
	@echo "Running govulncheck..."
	@tmp=$$(mktemp); \
	go run golang.org/x/vuln/cmd/govulncheck@latest ./... >"$$tmp" 2>&1 || true; \
	output=$$(grep -v '^exit status [0-9]*$$' "$$tmp" || true); \
	rm -f "$$tmp"; \
	ignored_ids=$$(grep 'id:' .govulncheck-ignore.yaml 2>/dev/null | cut -d'"' -f2); \
	total_vulns=$$(echo "$$output" | grep -c 'Vulnerability #' || true); \
	matching=0; \
	for id in $$ignored_ids; do \
		c=$$(echo "$$output" | grep -c "^Vulnerability.*$$id" || true); \
		matching=$$((matching + c)); \
	done; \
	if [ $$total_vulns -eq 0 ]; then \
		echo "$$output"; \
		echo ""; \
		echo "=== security: PASS (govulncheck clean) ==="; \
	elif [ $$total_vulns -eq $$matching ]; then \
		echo "$$output"; \
		echo ""; \
		echo "=== security: PASS (known false positives only) ==="; \
		echo "govulncheck reported $$matching advisories filtered — see .govulncheck-ignore.yaml."; \
	else \
		echo "$$output"; \
		echo ""; \
		echo "ERROR: $$((total_vulns - matching)) unfiltered govulncheck finding(s)."; \
		echo "Add to .govulncheck-ignore.yaml only with documented false-positive rationale."; \
		exit 1; \
	fi

security: govulncheck gocyclo grype

.PHONY: release-check
release-check:
	@set -e; \
	test -f VERSION || { echo "Error: VERSION file is required"; exit 1; }; \
	ver_raw=$$(cat VERSION | tr -d '\n\r'); ver=$${ver_raw#v}; \
	echo "$$ver" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "Error: VERSION must be semantic MAJOR.MINOR.PATCH (got: $$ver_raw)"; exit 1; }; \
	echo "Release version: $$ver (tag v$$ver)"
	@$(MAKE) lint
	@$(MAKE) test
	@$(MAKE) cover-check
	@$(MAKE) security
	@$(MAKE) docker-scan
	@echo "All release checks passed."

release: release-check
	@branch=$$(git branch --show-current 2>/dev/null); \
	if [ "$$branch" != "main" ]; then \
	  echo "Error: release only from main (current: $$branch). Merge develop → main first."; \
	  exit 1; \
	fi; \
	goreleaser release --clean

snapshot:
	@ver_raw=$$(cat VERSION 2>/dev/null | tr -d '\n\r'); \
	[ -n "$$ver_raw" ] || { echo "Error: VERSION file is required for snapshot"; exit 1; }; \
	ver=$${ver_raw#v}; \
	echo "$$ver" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "Error: VERSION must be semantic MAJOR.MINOR.PATCH (got: $$ver_raw)"; exit 1; }; \
	KPM_SNAPSHOT_VERSION="$$ver-next" goreleaser release --snapshot --clean

.PHONY: port-freebsd-sync
port-freebsd-sync:
	@[ -n "$(PORT_VERSION)" ] || { echo "Error: VERSION file empty or missing"; exit 1; }
	@sed -i.bak "s/^PORTVERSION=.*/PORTVERSION=\t$(PORT_VERSION)/" contrib/freebsd/Makefile
	@rm -f contrib/freebsd/Makefile.bak
	@echo "Updated contrib/freebsd/Makefile PORTVERSION to $(PORT_VERSION)"

.PHONY: port-openbsd-sync
port-openbsd-sync:
	@[ -n "$(PORT_VERSION)" ] || { echo "Error: VERSION file empty or missing"; exit 1; }
	@test -f contrib/openbsd/port/Makefile || { echo "Error: contrib/openbsd/port/Makefile not found"; exit 1; }
	@sed -i.bak \
	  -e 's#^DISTNAME =.*#DISTNAME =	kpm_v$(PORT_VERSION)_openbsd_$${MACHINE_ARCH:S/aarch64/arm64/}#' \
	  -e 's#^PKGNAME =.*#PKGNAME =	kpm-$(PORT_VERSION)#' \
	  -e 's#^MASTER_SITES =.*#MASTER_SITES =	https://github.com/hrodrig/kpm/releases/download/v$(PORT_VERSION)/#' \
	  -e 's#^DISTFILES =.*#DISTFILES =	kpm_v$(PORT_VERSION)_openbsd_$${MACHINE_ARCH:S/aarch64/arm64/}.tar.gz#' \
	  contrib/openbsd/port/Makefile
	@rm -f contrib/openbsd/port/Makefile.bak
	@echo "Updated contrib/openbsd/port/Makefile to $(PORT_VERSION)"

.PHONY: dist-freebsd
dist-freebsd:
	@set -e; \
	ver_raw=$$(cat VERSION 2>/dev/null | tr -d '\n\r'); \
	[ -n "$$ver_raw" ] || { echo "Error: VERSION file is required"; exit 1; }; \
	ver=$${ver_raw#v}; \
	echo "$$ver" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "Error: VERSION must be semantic MAJOR.MINOR.PATCH (got: $$ver_raw)"; exit 1; }; \
	echo "$(FREEBSD_ARCH)" | grep -qE '^(amd64|arm64)$$' || { echo "Error: FREEBSD_ARCH must be amd64 or arm64"; exit 1; }; \
	arch="$(FREEBSD_ARCH)"; \
	out="$(DIST)/kpm_v$${ver}_freebsd_$$arch.tar.gz"; \
	stage="/tmp/kpm-dist-root-$$PPID"; \
	tmpbin="$(DIST)/kpm-freebsd-$$arch-$$PPID"; \
	echo "Building kpm for FreeBSD $$arch with VERSION=v$$ver..."; \
	mkdir -p "$(DIST)"; \
	GOOS=freebsd GOARCH="$$arch" go build -trimpath $(LDFLAGS) -o "$$tmpbin" ./cmd/kpm; \
	rm -rf "$$stage"; \
	mkdir -p "$$stage/share/man/man1" "$$stage/share/doc/kpm" "$$stage/share/examples/kpm"; \
	cp "$$tmpbin" "$$stage/kpm"; \
	cp "$$tmpbin" "$$stage/kubectl-kpm"; \
	rm -f "$$tmpbin"; \
	cp LICENSE "$$stage/share/doc/kpm/LICENSE"; \
	cp configs/kpm.sample.yml "$$stage/share/examples/kpm/kpm.sample.yml"; \
	cp contrib/man/man1/kpm.1 "$$stage/share/man/man1/kpm.1"; \
	cp contrib/man/man1/kubectl-kpm.1 "$$stage/share/man/man1/kubectl-kpm.1"; \
	tar -C "$$stage" -czf "$$out" .; \
	rm -rf "$$stage"; \
	echo "Wrote $$out"

.PHONY: dist-openbsd
dist-openbsd:
	@set -e; \
	ver_raw=$$(cat VERSION 2>/dev/null | tr -d '\n\r'); \
	[ -n "$$ver_raw" ] || { echo "Error: VERSION file is required"; exit 1; }; \
	ver=$${ver_raw#v}; \
	echo "$$ver" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "Error: VERSION must be semantic MAJOR.MINOR.PATCH (got: $$ver_raw)"; exit 1; }; \
	echo "$(OPENBSD_ARCH)" | grep -qE '^(amd64|arm64)$$' || { echo "Error: OPENBSD_ARCH must be amd64 or arm64"; exit 1; }; \
	arch="$(OPENBSD_ARCH)"; \
	out="$(DIST)/kpm_v$${ver}_openbsd_$$arch.tar.gz"; \
	stage="/tmp/kpm-openbsd-dist-root-$$PPID"; \
	tmpbin="$(DIST)/kpm-openbsd-$$arch-$$PPID"; \
	echo "Building kpm for OpenBSD $$arch with VERSION=v$$ver..."; \
	mkdir -p "$(DIST)"; \
	GOOS=openbsd GOARCH="$$arch" go build -trimpath $(LDFLAGS) -o "$$tmpbin" ./cmd/kpm; \
	rm -rf "$$stage"; \
	mkdir -p "$$stage/share/man/man1" "$$stage/share/doc/kpm" "$$stage/share/examples/kpm"; \
	cp "$$tmpbin" "$$stage/kpm"; \
	cp "$$tmpbin" "$$stage/kubectl-kpm"; \
	rm -f "$$tmpbin"; \
	cp LICENSE "$$stage/share/doc/kpm/LICENSE"; \
	cp configs/kpm.sample.yml "$$stage/share/examples/kpm/kpm.sample.yml"; \
	cp contrib/man/man1/kpm.1 "$$stage/share/man/man1/kpm.1"; \
	cp contrib/man/man1/kubectl-kpm.1 "$$stage/share/man/man1/kubectl-kpm.1"; \
	tar -C "$$stage" -czf "$$out" .; \
	rm -rf "$$stage"; \
	echo "Wrote $$out"
