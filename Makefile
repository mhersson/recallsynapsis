.PHONY: help build build-go install web-install web-build web-check test test-race vet lint fmt fmt-check tidy-check scan scan-commits size check

# Stamps the version recallsynapsis prints; without it a build prints the commit.
VERSION ?=
LDFLAGS = -s -w$(if $(VERSION), -X main.version=$(VERSION))

help:
	@echo "Targets:"
	@echo "  help         show this list"
	@echo "  build        build the web app, then bin/recallsynapsis with it embedded"
	@echo "  build-go     build bin/recallsynapsis with the web app as built last, or the placeholder page when none is"
	@echo "  install      build the web app, then install recallsynapsis with it embedded into GOBIN, else the GOPATH bin directory"
	@echo "  web-install  install the web app's npm packages; needs the Node of .nvmrc"
	@echo "  web-build    build the web app into web/dist/app"
	@echo "  web-check    lint, test and build the web app"
	@echo "  test         run the tests"
	@echo "  test-race    run the tests with the race detector"
	@echo "  vet          go vet"
	@echo "  lint         golangci-lint run"
	@echo "  fmt          format the code with golangci-lint fmt"
	@echo "  fmt-check    fail and show the diff if formatting is needed"
	@echo "  tidy-check   fail if go mod tidy would change go.mod or go.sum"
	@echo "  scan         fail on em-dashes in any tracked or untracked, unignored text file"
	@echo "  scan-commits fail on session links, attribution, task, step or milestone numbers, em-dashes and big blobs in SCAN_BASE..SCAN_HEAD (origin/main..HEAD)"
	@echo "  size         count the non-test Go lines"
	@echo "  check        build-go, fmt-check, vet, lint, test-race, tidy-check, scan, size"
	@echo "VERSION=x.y.z on a build target stamps the version recallsynapsis prints."
	@echo "test and test-race run at GOMAXPROCS=4 unless GOMAXPROCS is given."

web-install:
	cd web && npm ci

web-build:
	cd web && npm run build

web-check:
	cd web && npm run lint && npm test && npm run build

# The web app first, even under make -j.
build: web-build
	$(MAKE) build-go

build-go:
	CGO_ENABLED=0 go build -trimpath -ldflags='$(LDFLAGS)' -o bin/recallsynapsis ./cmd/recallsynapsis

install: web-build
	CGO_ENABLED=0 go install -trimpath -ldflags='$(LDFLAGS)' ./cmd/recallsynapsis

# Go runs as many packages, and tests in each, at once as there are cores, and
# each test sign-in is a memory-hard argon2 verify; many cores exhaust memory.
test test-race: export GOMAXPROCS ?= 4

test:
	go test ./...

test-race:
	CGO_ENABLED=1 go test -race ./...

vet:
	go vet ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

fmt-check:
	@out=$$(golangci-lint fmt --diff 2>&1) && rc=0 || rc=$$?; \
	if [ $$rc -ne 0 ] || [ -n "$$out" ]; then \
		printf '%s\n' "$$out"; \
		echo "fmt-check: formatting needed; run 'make fmt'"; \
		exit 1; \
	fi

tidy-check:
	go mod tidy -diff

scan:
	@sh scripts/scan.sh

scan-commits:
	@sh scripts/scan-commits.sh "$(SCAN_BASE)" "$(SCAN_HEAD)"

size:
	@sh scripts/size.sh

check: build-go fmt-check vet lint test-race tidy-check scan size
