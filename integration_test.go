//go:build integration

package exoscale

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ego "github.com/exoscale/egoscale/v3"
	"github.com/libdns/libdns"
)

func liveProvider(t *testing.T) *Provider {
	t.Helper()
	key, secret := os.Getenv("EXOSCALE_API_KEY"), os.Getenv("EXOSCALE_API_SECRET")
	if key == "" || secret == "" {
		t.Skip("set EXOSCALE_API_KEY and EXOSCALE_API_SECRET to opt in")
	}
	return &Provider{APIKey: key, APISecret: secret}
}

func liveZone(t *testing.T) string {
	t.Helper()
	zone := strings.TrimSuffix(os.Getenv("EXOSCALE_TEST_ZONE"), ".")
	if zone == "" {
		t.Skip("set EXOSCALE_TEST_ZONE to a zone you control")
	}
	return zone
}

func TestLiveDNS(t *testing.T) {
	p, zone := liveProvider(t), liveZone(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := fmt.Sprintf("_caddy-test-%d", time.Now().UnixNano())
	records := []libdns.Record{libdns.TXT{Name: name, Text: "caddy-exoscale-one", TTL: time.Minute}, libdns.TXT{Name: name, Text: "caddy-exoscale-two", TTL: time.Minute}}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		if _, err := p.DeleteRecords(cleanup, zone, records); err != nil {
			t.Errorf("cleaning live DNS: %v", err)
		}
	})
	added, err := p.AppendRecords(ctx, zone, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 {
		t.Fatal("expected two TXT values")
	}
	deleted, err := p.DeleteRecords(ctx, zone, added[:1])
	if err != nil || len(deleted) != 1 {
		t.Fatalf("selective deletion: %v, %v", deleted, err)
	}
	deleted, err = p.DeleteRecords(ctx, zone, added[1:])
	if err != nil || len(deleted) != 1 {
		t.Fatalf("other value was not preserved: %v, %v", deleted, err)
	}
	deleted, err = p.DeleteRecords(ctx, zone, records)
	if err != nil || len(deleted) != 0 {
		t.Fatalf("cleanup not idempotent: %v, %v", deleted, err)
	}
	t.Log("real API: create, selective deletion, preserved sibling TXT, and repeated cleanup passed")
}

// TestLiveCertificate runs the built Caddy with DNS-01 as its only challenge.
// It issues both an ordinary and wildcard SAN with Let's Encrypt staging.
// No listener, deployed service, or existing certificate storage is required.
func TestLiveCertificate(t *testing.T) {
	_ = liveProvider(t)
	zone := liveZone(t)
	binary := os.Getenv("CADDY_BINARY")
	if binary == "" {
		t.Skip("set CADDY_BINARY to the absolute path of the built Caddy")
	}
	host := fmt.Sprintf("caddy-validation-%d.%s", time.Now().UnixNano(), zone)
	dir := t.TempDir()
	storage := filepath.Join(dir, "data")
	config := map[string]any{
		"admin":   map[string]any{"disabled": true},
		"storage": map[string]any{"module": "file_system", "root": storage},
		"apps": map[string]any{"tls": map[string]any{
			"certificates": map[string]any{"automate": []string{host, "*." + host}},
			"automation": map[string]any{"policies": []any{map[string]any{"issuers": []any{map[string]any{
				"module": "acme", "ca": "https://acme-staging-v02.api.letsencrypt.org/directory",
				"challenges": map[string]any{
					"http": map[string]any{"disabled": true}, "tls-alpn": map[string]any{"disabled": true},
					"dns": map[string]any{"provider": map[string]any{"name": "exoscale", "api_key": "{env.EXOSCALE_API_KEY}", "api_secret": "{env.EXOSCALE_API_SECRET}"}, "propagation_timeout": "5m", "resolvers": []string{"1.1.1.1:53", "8.8.8.8:53"}},
				},
			}}}}},
		}},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(dir, "caddy.json")
	if err := os.WriteFile(configPath, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(dir, "caddy.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "run", "--config", configPath)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	exited := false
	t.Cleanup(func() {
		cancel()
		if !exited {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Caddy process did not stop promptly")
			}
		}
		// Recover DNS challenges even if the Caddy process was interrupted.
		p := liveProvider(t)
		cleanup, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		id, err := p.initializeZone(cleanup, zone)
		if err != nil {
			t.Errorf("cleanup zone lookup: %v", err)
			return
		}
		recs, err := p.client.ListDNSDomainRecords(cleanup, id)
		if err != nil {
			t.Errorf("cleanup lookup: %v", err)
			return
		}
		name := "_acme-challenge." + strings.TrimSuffix(host, "."+zone)
		for _, r := range recs.DNSDomainRecords {
			if r.Name != name || string(r.Type) != "TXT" {
				continue
			}
			value, err := txtContent(r.Content)
			if err != nil {
				t.Error(err)
				continue
			}
			if _, err := p.DeleteRecords(cleanup, zone, []libdns.Record{libdns.TXT{Name: name, Text: value}}); err != nil {
				t.Errorf("challenge cleanup: %v", err)
			}
		}
	})
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	logText := func() string {
		data, _ := os.ReadFile(logFile.Name())
		text := string(data)
		for _, key := range []string{"EXOSCALE_API_KEY", "EXOSCALE_API_SECRET"} {
			if value := os.Getenv(key); value != "" {
				text = strings.ReplaceAll(text, value, "[REDACTED]")
			}
		}
		return text
	}
	for {
		found := make(map[string]bool)
		_ = filepath.WalkDir(storage, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".crt") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			block, _ := pem.Decode(data)
			if block == nil {
				return nil
			}
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil || time.Now().After(cert.NotAfter) || time.Now().Before(cert.NotBefore) {
				return nil
			}
			if !strings.Contains(strings.ToLower(cert.Issuer.String()), "staging") {
				return nil
			}
			for _, san := range cert.DNSNames {
				found[san] = true
			}
			return nil
		})
		if found[host] && found["*."+host] {
			t.Logf("Caddy issued valid Let's Encrypt staging certificates for %s and *.%s", host, host)
			// Allow Caddy to complete cleanup before stopping it.
			return
		}
		select {
		case err := <-done:
			exited = true
			t.Fatalf("Caddy exited before issuance: %v\n%s", err, logText())
		case <-ctx.Done():
			t.Fatalf("certificate issuance timed out: %s", logText())
		case <-ticker.C:
		}
	}
}

func (p *Provider) initializeZone(ctx context.Context, zone string) (id ego.UUID, err error) {
	if err = p.initialize(); err != nil {
		return "", err
	}
	return p.zoneID(ctx, zone)
}
