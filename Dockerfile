ARG CADDY_VERSION=2.11.8
FROM caddy:${CADDY_VERSION}-builder AS builder
ARG CADDY_VERSION
COPY . /src/caddy-exoscale
RUN xcaddy build v${CADDY_VERSION} \
    --with github.com/exogmi/caddy-exoscale=/src/caddy-exoscale

FROM caddy:${CADDY_VERSION}-alpine
LABEL org.opencontainers.image.source="https://github.com/exogmi/caddy-exoscale" \
      org.opencontainers.image.description="Caddy with the Exoscale DNS-01 certificate provider" \
      org.opencontainers.image.licenses="MIT"
COPY --from=builder /usr/bin/caddy /usr/bin/caddy
RUN apk add --no-cache ca-certificates curl
EXPOSE 80 443 443/udp
