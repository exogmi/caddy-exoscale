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

```sh
cp .env.example .env
# Fill in the DNS credentials locally, then:
docker compose up -d --build
```

The Dockerfile builds this checkout with xcaddy and Caddy 2.11.8. `.env` is
excluded from both Git and the Docker build context. Compose uses host networking
for Linux deployments and persistent certificate/configuration volumes.

For your existing stack, point the `caddy` build at this Dockerfile and keep the
existing `./caddy_data:/data` and `./caddy_config:/config` mounts to retain issued
certificates and ACME accounts. Keep the environment mapping from
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
