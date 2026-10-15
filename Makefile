.PHONY: build test test-integration lint vet fmt validate clean up down logs migrate migrate-status

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

# Integration tests start a disposable PostgreSQL through Testcontainers;
# Docker is their only prerequisite.
test-integration:
	go test -race -count=1 -tags integration ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -l -w .

validate: vet lint test build

clean:
	rm -rf bin dist .cache coverage.out

# Local stack (compose.yaml). VERSION, COMMIT and DATE reach the image builds
# as build args so the containers report the same identity as bin/.
export VERSION COMMIT DATE

.env:
	@echo "creating .env from .env.example with a generated database password"
	@pw=$$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n'); \
	sed -e "s/^FROODBASE_DB_PASSWORD=$$/FROODBASE_DB_PASSWORD=$$pw/" .env.example > .env

local/users.json:
	@echo "creating local/users.json from local/users.example.json (dev users, password froodbase-dev)"
	@cp local/users.example.json local/users.json

up: .env local/users.json
	docker compose up --build --wait

down:
	docker compose down

logs:
	docker compose logs --follow

# Migrations run from the api image inside the Compose network, so the
# container's FROODBASE_DATABASE_URL applies and nothing else is configured.
migrate: .env local/users.json
	docker compose run --rm --build migrate

migrate-status: .env local/users.json
	docker compose run --rm migrate migrate --status
