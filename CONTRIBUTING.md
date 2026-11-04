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

When updating Caddy, update its version in go.mod and the Dockerfile together,
run `go mod tidy`, verify the example configuration, and repeat staging issuance
before releasing. For a release, publish a semantic version tag after CI passes;
users can then pin that tag with xcaddy. Releases and container publication are
manual until maintainers choose their distribution policy.
