# Contributing

Use Go 1.26 or newer. Run `make test`, `make vet`, and `make build` before
submitting changes. Format Go sources with `gofmt` and Caddyfiles with
`./caddy fmt --overwrite examples/Caddyfile`.

Keep ordinary tests deterministic and independent of network access and real
credentials. Add regression coverage for DNS matching, cancellation, API error
handling, and configuration changes. Preserve unrelated TXT values during ACME
cleanup. Never record credentials, account keys, or certificate private keys in
fixtures, logs, screenshots, or commits.

The integration build tag enables opt-in tests against a user-owned Exoscale
zone and Let's Encrypt staging. See README.md for their environment variables.
Use a dedicated test key with DNS permissions and allow the tests to finish
cleanup. Normal CI compiles these tests but never supplies real credentials.

When updating Caddy, update its version in go.mod and the Dockerfile's runtime
image together, run `go mod tidy`, verify the
example configuration, and repeat staging issuance
before releasing. Every successful Release workflow on `main` creates the Git
version tag, GitHub release, and GHCR images automatically after testing both
container architectures. Do not create competing version tags manually.

Use `feat:` for a minor release, a conventional commit header with `!` or a
`BREAKING CHANGE:` footer for a major release, and ordinary commits for patch
releases. Version selection considers all commit messages since the latest
stable tag, starts at `v1.0.0`, and never increments again for a retry of an
already released commit. The module's version is independent of Caddy's version.
Use squash merge titles that preserve the intended conventional commit prefix.
For a major release, migrate the Go module path and imports (for example, add
`/v2`) in the breaking change; version validation rejects mismatched Go module
major versions before publication.

Run release version regression tests with:

```sh
python3 -m unittest discover -s .github/scripts -p 'test_*.py' -v
```

CI also checks workflow syntax with actionlint. Package publication uses the
repository's `GITHUB_TOKEN`; maintainers must set the new GHCR package to Public
after its first publication to enable anonymous pulls. No Exoscale credentials
are used during image builds or publication.

The Dockerfile compiles the module and Caddy directly from `go.mod` using the
Debian-based official Go image, and copies the binary into the official Caddy
Alpine runtime. Its configuration, CA certificates, and startup defaults are
inherited from that runtime image.
Maintain both Linux AMD64 and ARM64 targets. The shared container smoke check
is `.github/scripts/smoke-image.sh IMAGE ARCH`. It verifies the architecture,
Alpine identity, Caddy/module version, persistent storage paths, default
configuration, and example DNS configuration without credentials.

Workflows use standard GitHub-hosted Linux runners directly. The image smoke
test pipes example Caddyfile contents over stdin and requires no host bind mounts.
