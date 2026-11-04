#!/usr/bin/env bash
set -euo pipefail

image=${1:?usage: smoke-image.sh IMAGE ARCH}
arch=${2:?usage: smoke-image.sh IMAGE ARCH}

test "$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image")" = "linux/$arch"
docker run --rm "$image" sh -ec '. /etc/os-release; test "$ID" = alpine'
docker run --rm "$image" caddy list-modules | grep -Fx dns.providers.exoscale
expected=$(awk '$1 == "github.com/caddyserver/caddy/v2" {print $2}' go.mod)
actual=$(docker run --rm "$image" caddy version)
test "${actual%% *}" = "$expected"

# Keep the official Caddy image's persistent storage locations.
docker run --rm "$image" sh -ec 'test "$XDG_DATA_HOME" = /data; test "$XDG_CONFIG_HOME" = /config'
docker run --rm "$image" caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile

# Pipe config over stdin: a job-container workspace path is not necessarily
# present on the Docker daemon's host, so a host bind mount would be unreliable.
docker run --rm -i \
    -e EXOSCALE_API_KEY=mock-key -e EXOSCALE_API_SECRET=mock-secret \
    "$image" caddy adapt --config - --adapter caddyfile --validate \
    < examples/Caddyfile > /dev/null
