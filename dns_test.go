package exoscale

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	ego "github.com/exoscale/egoscale/v3"
	"github.com/libdns/libdns"
)

var errAPI = errors.New("mock API failure")

type mockDNS struct {
	records     []ego.DNSDomainRecord
	created     []ego.CreateDNSDomainRecordRequest
	deleted     []ego.UUID
	fail        string
	calls       int
	state       ego.OperationState
	missingZone bool
	onList      func(context.Context)
}

func (m *mockDNS) ListDNSDomains(ctx context.Context) (*ego.ListDNSDomainsResponse, error) {
	if m.onList != nil {
		m.onList(ctx)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.fail == "zones" {
		return nil, errAPI
	}
	if m.missingZone {
		return &ego.ListDNSDomainsResponse{}, nil
	}
	return &ego.ListDNSDomainsResponse{DNSDomains: []ego.DNSDomain{{ID: "zone-id", UnicodeName: "example.com"}}}, nil
}
func (m *mockDNS) ListDNSDomainRecords(ctx context.Context, id ego.UUID) (*ego.ListDNSDomainRecordsResponse, error) {
	if m.fail == "records" {
		return nil, errAPI
	}
	return &ego.ListDNSDomainRecordsResponse{DNSDomainRecords: m.records}, ctx.Err()
}
func (m *mockDNS) CreateDNSDomainRecord(ctx context.Context, id ego.UUID, r ego.CreateDNSDomainRecordRequest) (*ego.Operation, error) {
	m.calls++
	if m.fail == "create" || (m.fail == "second-create" && m.calls == 2) {
		return nil, errAPI
	}
	m.created = append(m.created, r)
	return &ego.Operation{State: ego.OperationStateSuccess}, ctx.Err()
}
func (m *mockDNS) DeleteDNSDomainRecord(ctx context.Context, id, record ego.UUID) (*ego.Operation, error) {
	if m.fail == "delete" {
		return nil, errAPI
	}
	if m.fail == "gone" {
		return nil, ego.ErrNotFound
	}
	m.deleted = append(m.deleted, record)
	return &ego.Operation{State: ego.OperationStateSuccess}, ctx.Err()
}
func (m *mockDNS) Wait(ctx context.Context, op *ego.Operation, states ...ego.OperationState) (*ego.Operation, error) {
	if m.fail == "wait" {
		return nil, errAPI
	}
	if m.fail == "nil-operation" {
		return nil, nil
	}
	if m.state != "" {
		return &ego.Operation{State: m.state}, nil
	}
	return op, ctx.Err()
}

func txt(value string) libdns.Record {
	return libdns.TXT{Name: "_acme-challenge", Text: value, TTL: time.Minute}
}
func apiTXT(id, value string) ego.DNSDomainRecord {
	return ego.DNSDomainRecord{ID: ego.UUID(id), Name: "_acme-challenge", Type: ego.DNSDomainRecordTypeTXT, Content: value, Ttl: 60}
}

func TestAppend(t *testing.T) {
	m := new(mockDNS)
	p := &Provider{client: m}
	got, err := p.AppendRecords(context.Background(), "EXAMPLE.com.", []libdns.Record{
		txt("first"), libdns.RR{Name: "_acme-challenge", Type: "TXT", Data: "second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(m.created) != 2 {
		t.Fatalf("records: %v", got)
	}
	for i, r := range m.created {
		if r.Ttl != 60 || r.Name != "_acme-challenge" || string(r.Type) != "TXT" {
			t.Fatalf("bad API request: %+v", r)
		}
		if got[i].RR().TTL != time.Minute {
			t.Fatal("incorrect TTL units")
		}
	}
}

func TestDeletePreservesOtherRecords(t *testing.T) {
	m := &mockDNS{records: []ego.DNSDomainRecord{
		apiTXT("own", `"my-token"`), apiTXT("other-order", `"other-token"`),
		apiTXT("chunks", `"my-" "token"`), apiTXT("other-ttl", "my-token"),
		{ID: "other-type", Name: "_acme-challenge", Type: ego.DNSDomainRecordTypeCNAME, Content: "my-token"},
		{ID: "other-name", Name: "other", Type: ego.DNSDomainRecordTypeTXT, Content: "my-token", Ttl: 60},
	}}
	m.records[3].Ttl = 120
	p := &Provider{client: m}
	got, err := p.DeleteRecords(context.Background(), "example.com.", []libdns.Record{txt("my-token"), txt("my-token"), txt("absent")})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || fmt.Sprint(m.deleted) != "[own chunks]" {
		t.Fatalf("deleted %v; returned %v", m.deleted, got)
	}
	if got[0].(libdns.TXT).TTL != time.Minute {
		t.Fatal("incorrect TTL")
	}
}

func TestDeleteApexAndZeroTTL(t *testing.T) {
	m := &mockDNS{records: []ego.DNSDomainRecord{{ID: "apex", Name: "", Type: ego.DNSDomainRecordTypeTXT, Content: "token", Ttl: 3600}}}
	p := &Provider{client: m}
	got, err := p.DeleteRecords(context.Background(), "example.com", []libdns.Record{libdns.TXT{Text: "token"}})
	if err != nil || len(got) != 1 || got[0].RR().TTL != time.Hour {
		t.Fatalf("%v, %v", got, err)
	}
	got, err = p.AppendRecords(context.Background(), "example.com", []libdns.Record{libdns.TXT{Text: "token"}})
	if err != nil || len(got) != 1 || m.created[0].Name != "@" {
		t.Fatalf("%v, %v", got, err)
	}
}

func TestInvalidBatchMakesNoChanges(t *testing.T) {
	for _, bad := range []libdns.Record{
		nil, libdns.RR{Type: "A", Data: "192.0.2.1"}, libdns.TXT{},
		libdns.TXT{Name: "bad..name", Text: "token"}, libdns.TXT{Name: "absolute.example.com.", Text: "token"},
		libdns.TXT{Text: "token", TTL: -time.Second}, libdns.TXT{Text: "token", TTL: time.Millisecond},
	} {
		for _, appendMode := range []bool{true, false} {
			m := new(mockDNS)
			p := &Provider{client: m}
			var err error
			if appendMode {
				_, err = p.AppendRecords(context.Background(), "example.com", []libdns.Record{txt("valid"), bad})
			} else {
				_, err = p.DeleteRecords(context.Background(), "example.com", []libdns.Record{txt("valid"), bad})
			}
			if err == nil || len(m.created)+len(m.deleted) != 0 {
				t.Fatalf("bad record accepted: %#v", bad)
			}
		}
	}
}

func TestFailures(t *testing.T) {
	for _, mode := range []string{"append", "delete"} {
		for _, fail := range []string{"zones", "wait", "nil-operation", "failed-state", "create", "records", "delete"} {
			if mode == "append" && (fail == "records" || fail == "delete") {
				continue
			}
			if mode == "delete" && fail == "create" {
				continue
			}
			t.Run(mode+"/"+fail, func(t *testing.T) {
				m := &mockDNS{fail: fail, records: []ego.DNSDomainRecord{apiTXT("id", "token")}}
				if fail == "failed-state" {
					m.state = ego.OperationStateFailure
				}
				p := &Provider{client: m}
				var err error
				if mode == "append" {
					_, err = p.AppendRecords(context.Background(), "example.com", []libdns.Record{txt("token")})
				} else {
					_, err = p.DeleteRecords(context.Background(), "example.com", []libdns.Record{txt("token")})
				}
				if err == nil {
					t.Fatal("expected failure")
				}
				if fail == "zones" || fail == "wait" || fail == "create" || fail == "records" || fail == "delete" {
					if !errors.Is(err, errAPI) {
						t.Fatalf("lost error cause: %v", err)
					}
				}
			})
		}
	}
}

func TestPartialSuccess(t *testing.T) {
	m := &mockDNS{fail: "second-create"}
	got, err := (&Provider{client: m}).AppendRecords(context.Background(), "example.com", []libdns.Record{txt("first"), txt("second")})
	if !errors.Is(err, errAPI) || len(got) != 1 || got[0].RR().Data != "first" {
		t.Fatalf("%v, %v", got, err)
	}
}

func TestZoneAndEmptyOperations(t *testing.T) {
	p := &Provider{client: &mockDNS{missingZone: true}}
	if _, err := p.AppendRecords(context.Background(), "example.com", []libdns.Record{txt("value")}); !errors.Is(err, ego.ErrNotFound) {
		t.Fatalf("%v", err)
	}
	if _, err := p.AppendRecords(context.Background(), "", []libdns.Record{txt("value")}); err == nil {
		t.Fatal("invalid zone")
	}
	if got, err := new(Provider).AppendRecords(context.Background(), "", nil); err != nil || len(got) != 0 {
		t.Fatalf("%v, %v", got, err)
	}
	if got, err := new(Provider).DeleteRecords(context.Background(), "", nil); err != nil || len(got) != 0 {
		t.Fatalf("%v, %v", got, err)
	}
	if _, err := new(Provider).AppendRecords(context.Background(), "example.com", []libdns.Record{txt("value")}); err == nil {
		t.Fatal("missing credentials")
	}
}

func TestCleanupMissingAndMalformed(t *testing.T) {
	for _, fail := range []string{"", "gone"} {
		m := &mockDNS{fail: fail}
		if fail == "gone" {
			m.records = []ego.DNSDomainRecord{apiTXT("id", "value")}
		}
		got, err := (&Provider{client: m}).DeleteRecords(context.Background(), "example.com", []libdns.Record{txt("value")})
		if err != nil || len(got) != 0 {
			t.Fatalf("%v, %v", got, err)
		}
	}
	m := &mockDNS{records: []ego.DNSDomainRecord{apiTXT("id", `"unterminated`)}}
	if _, err := (&Provider{client: m}).DeleteRecords(context.Background(), "example.com", []libdns.Record{txt("value")}); err == nil {
		t.Fatal("malformed TXT should fail")
	}
}

func TestTXTContent(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"raw", "raw"}, {`"quoted"`, "quoted"}, {`"a" "b"`, "ab"}, {`"a\"b"`, `a"b`}, {`"a\032b"`, "a b"}, {`"a\\b"`, `a\b`}} {
		got, err := txtContent(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("%q -> %q, want %q; %v", tc.in, got, tc.want, err)
		}
	}
}

func TestCancellationAndConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := new(Provider).AppendRecords(ctx, "example.com", []libdns.Record{txt("value")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
	m := new(mockDNS)
	p := &Provider{client: m}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Go(func() {
			if _, err := p.AppendRecords(context.Background(), "example.com", []libdns.Record{txt("value")}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if len(m.created) != 20 {
		t.Fatal("lost concurrent writes")
	}
	if err := p.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCtx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if err := p.lock(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("%v", err)
	}
	<-p.gate
	ctx, cancel = context.WithCancel(context.Background())
	m.onList = func(c context.Context) {
		if c != ctx {
			t.Error("context replaced")
		}
		cancel()
	}
	if _, err := p.AppendRecords(ctx, "example.com", []libdns.Record{txt("value")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("%v", err)
	}
}
