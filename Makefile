.PHONY: build test lint vet fmt validate clean

BINARIES := froodbase-mcp froodbase-api froodbase-dreamer
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -X github.com/rztaylor/froodbase/internal/version.Version=$(VERSION) \
            -X github.com/rztaylor/froodbase/internal/version.Commit=$(COMMIT) \
            -X github.com/rztaylor/froodbase/internal/version.Date=$(DATE)

build:
	@mkdir -p bin
	@for b in $(BINARIES); do \
		echo "building $$b"; \
		go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$$b ./cmd/$$b || exit 1; \
	done

test:
	go test -race -cover ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

validate: vet lint test build

clean:
	rm -rf bin dist .cache coverage.out
