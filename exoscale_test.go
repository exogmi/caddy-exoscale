package exoscale

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func TestModule(t *testing.T) {
	m, err := caddy.GetModule("dns.providers.exoscale")
	if err != nil {
		t.Fatal(err)
	}
	p := m.New().(*Provider)
	if err := json.Unmarshal([]byte(`{"api_key":"key","api_secret":"secret"}`), p); err != nil {
		t.Fatal(err)
	}
	if err := p.Provision(caddy.Context{Context: context.Background()}); err != nil {
		t.Fatal(err)
	}
}

func TestCaddyfile(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{"valid", "exoscale {\n api_key key\n api_secret secret\n}", true},
		{"placeholders", "exoscale {\n api_key {env.EXOSCALE_API_KEY}\n api_secret {env.EXOSCALE_API_SECRET}\n}", true},
		{"no key", "exoscale {\n api_secret secret\n}", false},
		{"no secret", "exoscale {\n api_key key\n}", false},
		{"arguments", "exoscale key secret", false},
		{"unknown", "exoscale {\n token value\n}", false},
		{"missing argument", "exoscale {\n api_key\n api_secret secret\n}", false},
		{"extra argument", "exoscale {\n api_key key extra\n api_secret secret\n}", false},
		{"duplicate", "exoscale {\n api_key key\n api_key key2\n api_secret secret\n}", false},
		{"nested", "exoscale {\n api_key key {\n nested value\n }\n api_secret secret\n}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p Provider
			err := p.UnmarshalCaddyfile(caddyfile.NewTestDispenser(tc.input))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}

func TestProvision(t *testing.T) {
	t.Setenv("CADDY_EXOSCALE_TEST_KEY", "test-key")
	t.Setenv("CADDY_EXOSCALE_TEST_SECRET", "test-secret")
	ctx := caddy.Context{Context: context.Background()}
	p := &Provider{APIKey: "{env.CADDY_EXOSCALE_TEST_KEY}", APISecret: "{env.CADDY_EXOSCALE_TEST_SECRET}"}
	if err := p.Provision(ctx); err != nil {
		t.Fatal(err)
	}
	if p.APIKey != "test-key" || p.APISecret != "test-secret" {
		t.Fatal("placeholders not expanded")
	}
	for _, p := range []*Provider{
		{}, {APIKey: "key"}, {APIKey: " ", APISecret: "secret"},
		{APIKey: "{env.CADDY_EXOSCALE_MISSING}", APISecret: "secret"},
		{APIKey: "key", APISecret: "{unknown.placeholder}"},
	} {
		if err := p.Provision(ctx); err == nil {
			t.Fatal("expected invalid credentials to fail")
		}
	}
	if err := (&Provider{APIKey: "key", APISecret: "secret"}).initialize(); err != nil {
		t.Fatal(err)
	}
	if err := new(Provider).initialize(); err == nil {
		t.Fatal("expected missing credentials error")
	}
	encoded, err := json.Marshal(&Provider{APIKey: "{env.KEY}", APISecret: "{env.SECRET}"})
	if err != nil || strings.Contains(string(encoded), "client") {
		t.Fatalf("unexpected JSON: %s, %v", encoded, err)
	}
}
