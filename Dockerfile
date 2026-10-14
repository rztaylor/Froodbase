# syntax=docker/dockerfile:1
# Builds one server binary (froodbase-api or froodbase-dreamer) into a small
# Alpine image. BINARY selects the cmd/ package; VERSION, COMMIT and DATE are
# the same link-time values the Makefile injects (see LDFLAGS there).
ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.22

FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS build
ARG BINARY
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
WORKDIR /src
COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X github.com/rztaylor/froodbase/internal/version.Version=${VERSION} \
                -X github.com/rztaylor/froodbase/internal/version.Commit=${COMMIT} \
                -X github.com/rztaylor/froodbase/internal/version.Date=${DATE}" \
      -o /out/froodbase ./cmd/${BINARY}

FROM alpine:${ALPINE_VERSION}
# busybox wget serves the Compose health check; ca-certificates for outbound TLS later.
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 froodbase
COPY --from=build /out/froodbase /usr/local/bin/froodbase
USER froodbase
ENTRYPOINT ["/usr/local/bin/froodbase"]
