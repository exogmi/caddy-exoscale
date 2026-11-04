# Validation record

Validated on 2026-11-04 using Go 1.27.3 on macOS arm64.

## Offline and mocked checks

* `go test -race -coverprofile=coverage.out ./...`: passed; provider package
  statement coverage **97.1%** (96.5% including the CLI entry point).
* `go vet ./...`: passed.
* `go mod verify`: passed.
* Go formatting and `git diff --check`: passed.
* Built `./cmd/caddy` reports **Caddy v2.11.8** and registers
  `dns.providers.exoscale`.
* The example Caddyfile adapts and validates with dummy environment credentials,
  without Exoscale API access.
* Integration tests compile and skip when opt-in credentials are absent.
* Credential scan found no values from the local `.env` in repository candidate
  files. `.env`, the local binary, and coverage output are ignored by Git;
  credentials are also excluded from the Docker build context.

## Real Exoscale API and Let's Encrypt staging

Final run against zone `containerstack.net`, using the built Caddy with module
path `github.com/exogmi/caddy-exoscale`:

```text
TestLiveDNS          PASS (104.09s)
TestLiveCertificate  PASS (13.50s)
```

The DNS lifecycle test created two TXT values under a unique name, deleted one,
verified that the sibling remained available for deletion, then verified repeated
cleanup was harmless. Cleanup ran with an independent bounded context.

The certificate test started an isolated Caddy process with DNS-01 as its only
enabled ACME challenge. Let's Encrypt staging issued certificates for:

* `caddy-validation-1793824542625442000.containerstack.net`
* `*.caddy-validation-1793824542625442000.containerstack.net`

The test checked the certificate SANs, validity dates, and staging issuer. It
then stopped Caddy and completed challenge-record cleanup. Certificate and ACME
account storage were temporary; no deployed services or existing certificate
storage were changed. Staging certificates are not publicly trusted.

## Packaging limits

The initial Docker recipe used upstream Caddy images. A subsequent hosted run
failed resolving the builder's platform manifest. That recipe has been replaced
by a direct Go build and a Debian slim runtime; see the Debian packaging update
below. The local Docker daemon remains unavailable, so a complete container
build must pass hosted CI before deployment.

Production CA issuance and long-running renewal were not exercised. They use
the same DNS-01 provider paths validated above.

## Release automation

Added and checked locally on 2026-11-04:

* actionlint v1.7.12 validates CI, the reusable workflow call, and Release.
* Ten Python regression tests pass for initial/patch/minor/major versioning,
  numerical tag ordering, invalid/prerelease tags, retry reuse, stale releases,
  GitHub output generation using real Git history, and Go major import paths.
* Go race tests and `go vet ./...` still pass.
* Both the default published-image Compose configuration and the local-build
  override pass `docker compose config --quiet`.
* Release builds and smoke tests native AMD64 and ARM64 images, then publishes
  versioned manifests, a Git/GitHub release, and `latest`. These hosted registry
  operations have not run locally; the repository must first be pushed.

After the first successful hosted release, set the GHCR package visibility to
Public and verify an anonymous pull before deploying the published image.

## Debian packaging update

Pulled the merged Dependabot updates from `main` through `5aff2b4` and retained
their newer GitHub Action versions. The Dockerfile now builds with
`golang:1.27.3-trixie` and runs on `debian:trixie-slim`. It installs only CA
certificates at runtime and preserves the Caddy command and storage conventions.

Both replacement image indexes were checked directly through the Docker
registry API and contain Linux AMD64 and ARM64 manifests. The old Caddy builder
index currently also advertises both platforms; the reported failure cannot
be reproduced from current registry metadata alone. The new build avoids that
upstream Caddy image dependency and explicitly selects its target platforms.

The exact stripped/static Go build commands succeeded for both Linux targets:

* AMD64: ELF x86-64, statically linked, stripped.
* ARM64: ELF aarch64, statically linked, stripped.

Go race tests, `go vet`, all ten release-version tests, actionlint, and shell
syntax validation pass. The default Caddyfile validates. Hosted smoke checks
now also assert Debian identity, architecture, storage environment, and default
configuration, using stdin for example configuration to support job containers.
The full Debian image build and smoke tests remain to be run in hosted CI.

All CI job runtimes now use Debian slim containers, with only the Docker client
installed where needed. GitHub-hosted Linux VMs themselves remain Ubuntu;
self-hosted Debian machines would be required to remove that host dependency.
