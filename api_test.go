package exoscale

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ego "github.com/exoscale/egoscale/v3"
	"github.com/exoscale/egoscale/v3/credentials"
	"github.com/libdns/libdns"
)

// Exercise the real SDK's signing, wire format, asynchronous operations and
// quoted TXT responses against a local HTTP server, without real credentials.
func TestSDKHTTPRoundTrip(t *testing.T) {
	const zoneID = "00000000-0000-4000-8000-000000000001"
	const recordID = "00000000-0000-4000-8000-000000000002"
	created, deleted := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !strings.Contains(r.Header.Get("Authorization"), "EXO2-HMAC-SHA256") {
			t.Error("missing Exoscale signature")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /dns-domain":
			_, _ = w.Write([]byte(`{"dns-domains":[{"id":"` + zoneID + `","unicode-name":"example.com"}]}`))
		case "POST /dns-domain/" + zoneID + "/record":
			var body ego.CreateDNSDomainRecordRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Content != "token" || body.Ttl != 60 || body.Name != "_acme-challenge" {
				t.Errorf("bad request: %+v", body)
			}
			created = true
			_, _ = w.Write([]byte(`{"id":"00000000-0000-4000-8000-000000000003","state":"pending"}`))
		case "GET /operation/00000000-0000-4000-8000-000000000003":
			_, _ = w.Write([]byte(`{"id":"00000000-0000-4000-8000-000000000003","state":"success"}`))
		case "GET /dns-domain/" + zoneID + "/record":
			_, _ = w.Write([]byte(`{"dns-domain-records":[{"id":"` + recordID + `","name":"_acme-challenge","type":"TXT","content":"\"token\"","ttl":60}]}`))
		case "DELETE /dns-domain/" + zoneID + "/record/" + recordID:
			deleted = true
			_, _ = w.Write([]byte(`{"state":"success"}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := ego.NewClient(credentials.NewStaticCredentials("mock-key", "mock-secret"), ego.ClientOptWithEndpoint(ego.Endpoint(server.URL)))
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{client: client}
	recs, err := p.AppendRecords(context.Background(), "example.com.", []libdns.Record{txt("token")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DeleteRecords(context.Background(), "example.com.", recs); err != nil {
		t.Fatal(err)
	}
	if !created || !deleted {
		t.Fatal("incomplete DNS lifecycle")
	}
}
