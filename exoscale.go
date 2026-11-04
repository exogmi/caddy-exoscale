// Package exoscale implements the dns.providers.exoscale Caddy module for
// ACME DNS-01 challenges using the Exoscale API.
package exoscale

import (
	"fmt"
	"strings"
	"sync"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	ego "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
)

// Provider adds and removes TXT records for certificate issuance. Configure
// credentials before use; a Provider must not be copied after first use.
type Provider struct {
	APIKey    string `json:"api_key,omitempty"`
	APISecret string `json:"api_secret,omitempty"`

	once    sync.Once
	client  dnsClient
	initErr error
	// Serialize DNS operations within this provider without blocking cancellation.
	gate chan struct{}
}

func init() { caddy.RegisterModule(new(Provider)) }

// CaddyModule returns the module registration, preserving the original name.
func (*Provider) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{ID: "dns.providers.exoscale", New: func() caddy.Module { return new(Provider) }}
}

// Provision resolves runtime placeholders and validates credentials without
// making API requests. Environment placeholders keep secrets out of adapted JSON.
func (p *Provider) Provision(_ caddy.Context) error {
	r := caddy.NewReplacer()
	var err error
	p.APIKey, err = r.ReplaceOrErr(p.APIKey, true, true)
	if err != nil {
		return fmt.Errorf("exoscale: resolving api_key: %w", err)
	}
	p.APISecret, err = r.ReplaceOrErr(p.APISecret, true, true)
	if err != nil {
		return fmt.Errorf("exoscale: resolving api_secret: %w", err)
	}
	return p.Validate()
}

// Validate checks required credentials without including their values in errors.
func (p *Provider) Validate() error {
	if strings.TrimSpace(p.APIKey) == "" {
		return fmt.Errorf("exoscale: api_key is required")
	}
	if strings.TrimSpace(p.APISecret) == "" {
		return fmt.Errorf("exoscale: api_secret is required")
	}
	return nil
}

func (p *Provider) initialize() error {
	p.once.Do(func() {
		p.gate = make(chan struct{}, 1)
		if p.client != nil {
			return
		} // Test clients use the same provider logic.
		if p.initErr = p.Validate(); p.initErr != nil {
			return
		}
		p.client, p.initErr = ego.NewClient(credentials.NewStaticCredentials(p.APIKey, p.APISecret),
			ego.ClientOptWithUserAgent("exoscale/caddy-exoscale"))
	})
	return p.initErr
}

// UnmarshalCaddyfile accepts: exoscale { api_key <key>; api_secret <secret> }.
func (p *Provider) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	seen := make(map[string]bool)
	for d.Next() {
		if d.NextArg() {
			return d.ArgErr()
		}
		for nesting := d.Nesting(); d.NextBlock(nesting); {
			name := d.Val()
			if name != "api_key" && name != "api_secret" {
				return d.Errf("unrecognized subdirective %q", name)
			}
			if seen[name] {
				return d.Errf("duplicate subdirective %q", name)
			}
			seen[name] = true
			var value string
			if !d.AllArgs(&value) {
				return d.ArgErr()
			}
			if d.NextBlock(d.Nesting()) {
				return d.Err("nested blocks are not supported")
			}
			if name == "api_key" {
				p.APIKey = value
			} else {
				p.APISecret = value
			}
		}
	}
	return p.Validate()
}

var (
	_ caddy.Module          = (*Provider)(nil)
	_ caddy.Provisioner     = (*Provider)(nil)
	_ caddy.Validator       = (*Provider)(nil)
	_ caddyfile.Unmarshaler = (*Provider)(nil)
)
