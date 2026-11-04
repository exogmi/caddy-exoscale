# syntax=docker/dockerfile:1
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

FROM debian:trixie-slim
LABEL org.opencontainers.image.source="https://github.com/exogmi/caddy-exoscale" \
      org.opencontainers.image.description="Caddy with the Exoscale DNS-01 certificate provider" \
      org.opencontainers.image.licenses="MIT"
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /etc/caddy /data/caddy /config/caddy /srv
ENV XDG_DATA_HOME=/data \
    XDG_CONFIG_HOME=/config
COPY --from=builder /out/caddy /usr/bin/caddy
COPY docker/Caddyfile /etc/caddy/Caddyfile
WORKDIR /srv
EXPOSE 80 443 443/udp
STOPSIGNAL SIGTERM
CMD ["caddy", "run", "--config", "/etc/caddy/Caddyfile", "--adapter", "caddyfile"]
