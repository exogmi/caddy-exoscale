# syntax=docker/dockerfile:1
ARG CADDY_VERSION=2.11.8
# Compile natively on the builder; BuildKit supplies the target architecture.
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/caddy ./cmd/caddy

FROM caddy:${CADDY_VERSION}-alpine
LABEL org.opencontainers.image.source="https://github.com/exogmi/caddy-exoscale" \
      org.opencontainers.image.description="Caddy with the Exoscale DNS-01 certificate provider" \
      org.opencontainers.image.licenses="MIT"
COPY --from=builder /out/caddy /usr/bin/caddy
