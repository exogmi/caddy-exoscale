# Caddy Exoscale DNS

Caddy DNS-01 module for Let's Encrypt certificates using Exoscale DNS, including
wildcard certificates. Targets **Caddy v2.11.8**, **libdns v1.1.1**, and the
official **egoscale v3.1.62** SDK. Requires Go 1.26 or newer.

The module name remains `dns.providers.exoscale`; existing `dns exoscale`
Caddyfiles work unchanged. Do not include the old caddy-dns/exoscale plugin in
the same binary: both register that module name.

## Build

Build from this checkout, including Caddy's standard modules:

```sh
make build
./caddy list-modules
```

Or use xcaddy with a local replacement:

```sh
xcaddy build v2.11.8 --with github.com/exogmi/caddy-exoscale=.
```

`github.com/exogmi/caddy-exoscale` is the repository/module path.
Remote `xcaddy --with` builds require the code to be pushed and a release tag.
This project has not yet been published as an official Exoscale release.

## Configuration

```caddyfile
coder.containerstack.net, *.coder.containerstack.net {
    reverse_proxy localhost:7080
    tls {
        dns exoscale {
            api_key {env.EXOSCALE_API_KEY}
            api_secret {env.EXOSCALE_API_SECRET}
        }
    }
}
```

Set `EXOSCALE_API_KEY` and `EXOSCALE_API_SECRET` in Caddy's environment. Runtime
`{env.*}` placeholders avoid embedding the actual values in adapted JSON. Your
existing `{$EXOSCALE_API_KEY}` and `{$EXOSCALE_API_SECRET}` syntax also works,
but expands credentials while adapting the Caddyfile.

Both options are required. Unknown directives, duplicate directives, missing or
extra arguments, nested blocks, and unresolved runtime placeholders are rejected.
JSON configuration uses `name: "exoscale"`, `api_key`, and `api_secret` in the
ACME DNS challenge provider object.

The key needs access to list DNS domains, list DNS records, create/delete DNS
records, and read asynchronous operations. Restrict it to the necessary DNS
zone where supported. No compute or instance permissions are needed. The SDK
uses Exoscale's default API endpoint and request signing.

## Docker and migration

Published releases are available as multi-platform images for Linux AMD64 and
ARM64, with Caddy and this module included:

```sh
docker pull ghcr.io/exogmi/caddy-exoscale:1.0.0
# Or follow the most recently completed release:
docker pull ghcr.io/exogmi/caddy-exoscale:latest
```

The first release is `1.0.0`; these images become available after the code is
pushed, the Release workflow passes, and the GHCR package is made public.
Pin a version or image digest for a controlled production rollout. Updating
`latest` in the registry does not automatically restart running containers.

```sh
cp .env.example .env
# Fill in the DNS credentials and desired image version locally, then:
docker compose pull
docker compose up -d
```

Compose uses `CADDY_EXOSCALE_VERSION` from `.env` (defaults to `latest` if unset).
The sample `.env.example` pins the initial `1.0.0` release. To build this checkout
locally instead:

```sh
docker compose -f compose.yaml -f compose.build.yaml up -d --build
```

The image uses **Debian 13 `trixie-slim`**, with only CA certificates added.
A separate Debian-based Go builder compiles a stripped, statically linked Caddy
binary directly from this repository. The Caddy version comes from `go.mod`;
there is no dependency on upstream Caddy builder/runtime image manifests.
BuildKit explicitly targets Linux AMD64 or ARM64, with module and compiler
caches. No Go compiler, Git, curl, or build tools are included in the runtime.

The image preserves `/etc/caddy/Caddyfile`, `/data`, `/config`, the standard
`caddy run` command, and ports 80/443 plus UDP 443. Without a mounted Caddyfile,
the default configuration serves a simple HTTP response on port 80. `.env` is
excluded from Git and the Docker build context. Compose uses host networking
for Linux deployments and persistent certificate/configuration volumes.

For your existing stack, replace the Caddy `build` section and local `image` with
`image: ghcr.io/exogmi/caddy-exoscale:1.0.0`. Keep the existing
`./caddy_data:/data` and `./caddy_config:/config` mounts to retain issued
certificates and ACME accounts, and keep the environment mapping from
`DNS_EXOSCALE_API_KEY/SECRET` to `EXOSCALE_API_KEY/SECRET`. You can retain your
existing Caddyfile, bind addresses, and reverse proxies. The standalone example
uses new named volumes; do not switch an existing deployment to those volumes
unless you intend to initialize new storage.

For initial validation add this global option to a test Caddyfile:

```caddyfile
{
    acme_ca https://acme-staging-v02.api.letsencrypt.org/directory
}
```

Staging certificates are deliberately untrusted. Remove the staging CA setting
for normal Let's Encrypt issuance. DNS-01 requires public authoritative DNS
propagation, but does not require inbound access to ports 80 or 443.

## Automated releases

Pull requests run CI without registry write access or real DNS credentials.
Pushes to `main` run the same CI before the Release workflow publishes images.
Both native Linux AMD64 and ARM64 images must build and pass module-registration,
Caddy-version, platform, Debian-runtime, storage-location, and Caddyfile-validation
checks before release tags are promoted. CI uses an explicit Linux platform and
the Buildx container driver to avoid implicit platform selection.

All job commands run in Debian slim containers, including testing, versioning,
image building, and publication. GitHub's underlying hosted Linux machines
still use Ubuntu: GitHub does not offer a standard hosted Debian runner. A fully
Debian host requires self-hosted runners. The published image and its build
stages contain no Ubuntu packages. Docker jobs install only Debian's Docker
client and use the host daemon; they do not start a nested Docker daemon.

Versioning is automatic, based on existing Git tags:

* The first release is `v1.0.0`.
* Ordinary commits, including `fix:`, `docs:`, and `ci:`, increment the patch.
* `feat:` or `feat(scope):` increments the minor version.
* A conventional commit header containing `!`, or a `BREAKING CHANGE:` footer,
  increments the major version. The highest bump since the previous release wins.
* A successful release creates a matching Git tag and GitHub release, and image
  tags `1.0.0`, `v1.0.0`, and `latest` (using the computed version).

The module's release version is separate from the bundled Caddy version. A
major release requires its Go module/import paths to match that major
version (for example, `/v2` for `v2.0.0`). The workflow checks this before builds.
No version-bump commit is written back to `main`. Release runs are serialized;
superseded commits are skipped, and fast pushes may be coalesced by GitHub's
concurrency queue. The next successful release includes accumulated changes.
Re-running Release for an already released current commit reuses its version
and repairs release/latest publication without rebuilding its versioned images.
Per-architecture `build-*` registry tags are internal staging images and are not
supported release tags. Failed builds do not update `latest`.

Publishing uses the built-in `GITHUB_TOKEN` with job-scoped `packages: write` and
`contents: write`; no registry password or PAT is needed. Repository policy must
permit those permissions. After the first publication, open the repository's
**Packages → caddy-exoscale → Package settings → Change visibility → Public**.
GHCR initially creates packages as private, even for a public source repository;
public images can then be pulled anonymously. See
[GitHub's Container registry documentation](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).

Wait for the first successful Release run and verify an anonymous image pull
before deploying the published image. To retry a failed publication, use
**Actions → Release → Run workflow** on `main`.

## Record behavior

This is a certificate DNS provider, implementing libdns `RecordAppender` and
`RecordDeleter` for **TXT records only**. It does not expose zone management,
general DNS record listing, or destructive RRset replacement.

* Appends add TXT values without replacing existing values, allowing overlapping
  apex/wildcard challenges. A zero TTL defaults to 60 seconds.
* Deletion matches the relative name, TXT type, and exact text; nonzero TTLs must
  also match. Zero TTL matches any TTL. Missing records are ignored.
* Empty TXT values and other record types are rejected, including in deletion
  requests, to prevent broad cleanup. Names are relative to the supplied zone;
  `@` or the empty name denotes the apex. Zones may have a trailing dot.
* API TXT quoting and multiple quoted chunks are decoded before comparison.
* The entire input batch is validated before mutations. API operations are
  sequential and batches are not atomic: successful records are returned with
  an error if a later operation fails. A timed-out operation may still complete
  server-side; inspect isolated test records after interrupted validation.
* Calls are safe for concurrent use, including initialization. Waiting for the
  provider lock, zone lookups, record mutations, and operation polling honor
  cancellation. Credentials must not be changed after first use.
* SDK retry behavior is retained; the module adds no blind write retries.

## Tests

```sh
make test
make vet
```

Normal tests use in-memory mocks and a local HTTP server with the actual SDK;
they require no real credentials or access to Exoscale. They cover configuration,
runtime placeholders, module registration, signed SDK requests, asynchronous
operations, quoting, TTL conversion, overlapping TXT values, safe/idempotent
cleanup, batch validation, partial failures, cancellation, and concurrent use.

Live tests require explicit opt-in and are excluded from normal builds:

```sh
export EXOSCALE_API_KEY='your-key'
export EXOSCALE_API_SECRET='your-secret'
export EXOSCALE_TEST_ZONE=containerstack.net
make integration
```

`TestLiveDNS` creates a unique TXT name, verifies selective deletion preserves
another TXT value, then removes both. `TestLiveCertificate` runs the built Caddy
with DNS-01 as its only challenge against Let's Encrypt **staging**, checks valid
certificate files for a unique hostname and its wildcard, and cleans up that
hostname's challenge records. It uses temporary certificate/account storage and
never touches your deployed Caddy. Allow outbound HTTPS and DNS, including access
to public resolvers 1.1.1.1 and 8.8.8.8. Tests skip when opt-in variables are absent.

## Upstream and licensing

The original Caddy adapter was inspected from
https://github.com/caddy-dns/exoscale at commit
`cfc9a8211a2df59fad2b97ad6b30142061956de7`. Its underlying
https://github.com/libdns/exoscale provider still used the pre-v1 record struct
API. This implementation replaces that dependency with the official Exoscale
SDK and libdns v1 interfaces. The upstream MIT license and attribution are
preserved in [LICENSE](LICENSE).
