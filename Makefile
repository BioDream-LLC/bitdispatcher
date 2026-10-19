GO      ?= go
PNPM    ?= corepack pnpm
BIN     := bin/bitdispatcher
PKG     := ./cmd/bitdispatcher
# The released version, written here so a build from a source archive reports the same version as a checkout.
VERSION ?= v0.0.0
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build sim test vet fmt check web web-test manual e2e clean

all: check build

# The web interface is built into internal/web/dist and committed, so `go build` needs no Node.
build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) $(PKG)

# bvsim is a stand-in for BitVoice's CAD hooks, for demonstrations and tests without a phone system.
sim:
	CGO_ENABLED=0 $(GO) build -trimpath -o bin/bvsim ./cmd/bvsim

web:
	cd web && $(PNPM) install --frozen-lockfile && $(PNPM) build

web-test:
	cd web && $(PNPM) test

# e2e runs the browser suite (web/e2e/suite) against a real bitdispatcher with demo data and the
# BitVoice simulator.
e2e: build sim
	cd web && $(PNPM) exec playwright test

# manual prints the Help chapters (web/src/manual.ts and figures.ts) as docs/manual/index.html and a PDF.
manual:
	cd web && $(PNPM) exec tsc src/manual.ts src/figures.ts --outDir /tmp/bitdispatcher-manual --module es2022 --target es2022 --moduleResolution bundler --skipLibCheck && \
		cp /tmp/bitdispatcher-manual/manual.js /tmp/bitdispatcher-manual/manual.mjs && cp /tmp/bitdispatcher-manual/figures.js /tmp/bitdispatcher-manual/figures.mjs && \
		mkdir -p $(CURDIR)/docs/manual && node scripts/manual.mjs /tmp/bitdispatcher-manual/manual.mjs $(CURDIR)/docs/manual/index.html $(CURDIR)/docs/manual/bitdispatcher-manual.pdf && \
		cp $(CURDIR)/docs/manual/index.html $(CURDIR)/docs/manual/bitdispatcher-manual.pdf $(CURDIR)/internal/web/manual/

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
