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

Docker Hub confirms the pinned `caddy:2.11.8-builder` and
`caddy:2.11.8-alpine` tags exist. The local Docker daemon did not respond to its
availability check, so the container image was not built locally. The GitHub CI
workflow builds the Dockerfile and checks module registration in the image;
that hosted workflow will run after the repository is pushed.

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
