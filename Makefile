GO      ?= go
PNPM    ?= corepack pnpm
BIN     := bin/bitdispatcher
PKG     := ./cmd/bitdispatcher
# The released version, written here so a build from a source archive reports the same version as a checkout.
VERSION ?= v0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build fips arm64 windows linux macos sim test test-fips vet vuln fmt check web web-test manual e2e site clean social-card

all: check build

# The web interface is built into internal/web/dist and committed, so `go build` needs no Node.
build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

# fips builds with the Go Cryptographic Module v1.0.0, which holds FIPS 140-3 certificate #5247, and runs
# in FIPS 140-3 mode. GODEBUG=fips140=only at run time also refuses every non-approved algorithm.
fips:
	GOFIPS140=v1.0.0 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

# arm64 builds for 64-bit ARM Linux, such as a Raspberry Pi 5 kept as a second site (FIPS build, as above).
arm64:
	GOOS=linux GOARCH=arm64 GOFIPS140=v1.0.0 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/bitdispatcher-linux-arm64 $(PKG)

# windows builds the Windows program (FIPS build, with its icon and version) and its installer,
# bin/bitdispatcher-setup.exe: a service started with Windows (packaging/windows). Needs NSIS 3 (makensis).
MAKENSIS ?= makensis
PKGVER   := $(patsubst v%,%,$(VERSION))
windows:
	cd cmd/bitdispatcher && $(GO) run github.com/tc-hib/go-winres@v0.3.3 simply --arch amd64 --icon ../../packaging/windows/icon-256.png \
		--product-name bitdispatcher --file-description "bitdispatcher computer-aided dispatch" \
		--copyright "Copyright BioDream LLC. Apache License 2.0." --file-version $(PKGVER).0 --product-version $(PKGVER).0 --manifest cli --out rsrc
	GOOS=windows GOARCH=amd64 GOFIPS140=v1.0.0 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/bitdispatcher-windows-amd64.exe $(PKG); \
		s=$$?; rm -f cmd/bitdispatcher/rsrc_windows_amd64.syso; exit $$s
	cd packaging/windows && $(MAKENSIS) -V2 -DVERSION=$(PKGVER) bitdispatcher.nsi

# linux builds, for Intel and Arm, the Linux download (bin/bitdispatcher-<version>-linux-<arch>.tar.gz,
# with install.sh) and .deb and .rpm packages: a hardened systemd service (packaging/linux). FIPS build.
NFPM := github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.48.0
linux:
	set -e; for a in amd64 arm64; do \
	  d=bin/bitdispatcher-$(PKGVER)-linux-$$a; rm -rf $$d; mkdir -p $$d; \
	  GOOS=linux GOARCH=$$a GOFIPS140=v1.0.0 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $$d/bitdispatcher $(PKG); \
	  cp packaging/linux/install.sh packaging/linux/uninstall.sh packaging/linux/postinstall.sh packaging/linux/bitdispatcher.service \
	    packaging/linux/bitdispatcher.default LICENSE docs/manual/bitdispatcher-manual.pdf $$d/; \
	  tar -C bin -czf $$d.tar.gz $${d#bin/}; \
	  mkdir -p bin/nfpm; cp $$d/bitdispatcher bin/nfpm/bitdispatcher; \
	  for f in deb rpm; do NFPM_ARCH=$$a NFPM_VERSION=$(PKGVER) $(GO) run $(NFPM) package -f packaging/linux/nfpm.yaml -p $$f -t bin/; done; \
	  rm -rf $$d bin/nfpm; \
	done

# macos builds bin/bitdispatcher-VERSION-macos.pkg (Apple silicon and Intel, macOS 12+): a launchd service
# under its own account, apps in Applications › bitdispatcher (packaging/macos). FIPS build. On a Mac only.
# Optional: APP_SIGN, PKG_SIGN (Developer ID identities) and NOTARY_PROFILE; see packaging/macos/build.sh.
macos:
	GO=$(GO) sh packaging/macos/build.sh $(PKGVER) "$(LDFLAGS)"

# test-fips runs the Go tests in FIPS 140-only mode: anything using a non-approved algorithm fails.
test-fips:
	GOFIPS140=v1.0.0 GODEBUG=fips140=only $(GO) test ./...

# vuln looks for known vulnerabilities in the Go code and the standard library it calls, and in the web
# interface's packages. govulncheck is pinned; install it with
#   go install golang.org/x/vuln/cmd/govulncheck@v1.9.0
vuln:
	govulncheck ./...
	cd web && $(PNPM) audit

# bvsim is a stand-in for BitVoice's CAD hooks, for demonstrations and tests without a phone system.
sim:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/bvsim ./cmd/bvsim
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/dronesim ./cmd/dronesim

web:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) build

web-test:
	cd web && $(PNPM) test

# e2e runs the browser suite (web/e2e/suite) against a real bitdispatcher with demo data and the
# BitVoice and drone simulators.
e2e: build sim
	cd web && $(PNPM) exec playwright test

# manual prints the Help chapters (web/src/manual.ts and figures.ts) as docs/manual/index.html and a PDF.
manual:
	cd web && $(PNPM) exec tsc src/manual.ts src/figures.ts --outDir /tmp/bitdispatcher-manual --module es2022 --target es2022 --moduleResolution bundler --skipLibCheck && \
		cp /tmp/bitdispatcher-manual/manual.js /tmp/bitdispatcher-manual/manual.mjs && cp /tmp/bitdispatcher-manual/figures.js /tmp/bitdispatcher-manual/figures.mjs && \
		mkdir -p $(CURDIR)/docs/manual && node scripts/manual.mjs /tmp/bitdispatcher-manual/manual.mjs $(CURDIR)/docs/manual $(CURDIR)/docs/manual/i18n && \
		cp $(CURDIR)/docs/manual/index.html $(CURDIR)/docs/manual/bitdispatcher-manual.pdf $(CURDIR)/internal/web/manual/ && \
		for f in $(CURDIR)/docs/manual/i18n/*.json; do l=$$(basename $$f .json); mkdir -p $(CURDIR)/internal/web/manual/$$l && cp $(CURDIR)/docs/manual/$$l/index.html $(CURDIR)/internal/web/manual/$$l/; done

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

# check fails on unformatted code rather than rewriting it.
check: vet test
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

clean:
	rm -rf bin

# The bitdispatcher.com website, built from README.md, docs/manual and docs/screenshots (site/ holds its
# pages). Publish it with deploy/site.sh.
social-card:
	cd web && node scripts/social-card.mjs $(CURDIR)

site:
	$(GO) run ./cmd/bitdispatchersite -out dist-site -base https://bitdispatcher.com
