# cubrid_exporter — build / test / multi-arch release.
# This module is self-contained (stdlib-only). The parent repo ships a go.work,
# so builds force GOWORK=off to keep this module independent.

BINARY  := cubrid_exporter
VERSION := 1.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)
GOENV   := GOWORK=off GOTOOLCHAIN=local CGO_ENABLED=0
PLATFORMS := linux/amd64 linux/arm64

.PHONY: all build test vet fmt release clean run-live

all: test build

build:
	$(GOENV) go build -ldflags='$(LDFLAGS)' -o $(BINARY) .

test:
	GOWORK=off GOTOOLCHAIN=local go test ./...

vet:
	GOWORK=off GOTOOLCHAIN=local go vet ./...

fmt:
	gofmt -w *.go

# Cross-compiled, stripped, static binaries + tarballs under dist/.
release: test
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os=$${p%/*}; arch=$${p#*/}; \
	  out=dist/$(BINARY)-$(VERSION).$$os-$$arch; \
	  echo "building $$out"; \
	  $(GOENV) GOOS=$$os GOARCH=$$arch go build -ldflags='$(LDFLAGS)' -o $$out/$(BINARY) . ; \
	  cp README.md TEST_RESULTS.md deploy/cubrid_exporter.service $$out/ ; \
	  tar -C dist -czf $$out.tar.gz $$(basename $$out) ; \
	  rm -rf $$out ; \
	done
	@echo "--- dist ---"; ls -la dist

run-live:
	./run/start-live.sh 9890 testdb,demodb

clean:
	rm -f $(BINARY); rm -rf dist
