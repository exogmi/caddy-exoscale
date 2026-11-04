package exoscale

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ego "github.com/exoscale/egoscale/v3"
	"github.com/libdns/libdns"
	"github.com/miekg/dns"
)

type dnsClient interface {
	ListDNSDomains(context.Context) (*ego.ListDNSDomainsResponse, error)
	ListDNSDomainRecords(context.Context, ego.UUID) (*ego.ListDNSDomainRecordsResponse, error)
	CreateDNSDomainRecord(context.Context, ego.UUID, ego.CreateDNSDomainRecordRequest) (*ego.Operation, error)
	DeleteDNSDomainRecord(context.Context, ego.UUID, ego.UUID) (*ego.Operation, error)
	Wait(context.Context, *ego.Operation, ...ego.OperationState) (*ego.Operation, error)
}

func (p *Provider) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.initialize(); err != nil {
		return err
	}
	select {
	case p.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-p.gate
			return err
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *Provider) zoneID(ctx context.Context, zone string) (ego.UUID, error) {
	zone = strings.TrimSuffix(zone, ".")
	if _, ok := dns.IsDomainName(zone); !ok || zone == "" {
		return "", fmt.Errorf("exoscale: invalid zone")
	}
	zones, err := p.client.ListDNSDomains(ctx)
	if err != nil {
		return "", fmt.Errorf("exoscale: listing zones: %w", err)
	}
	for _, z := range zones.DNSDomains {
		if strings.EqualFold(z.UnicodeName, zone) {
			return z.ID, nil
		}
	}
	return "", fmt.Errorf("exoscale: zone %q: %w", zone, ego.ErrNotFound)
}

// challengeRecords validates the entire batch before making any DNS changes.
func challengeRecords(records []libdns.Record) ([]libdns.TXT, error) {
	out := make([]libdns.TXT, 0, len(records))
	for _, record := range records {
		if record == nil {
			return nil, fmt.Errorf("exoscale: nil record")
		}
		r := record.RR()
		if r.Type != "TXT" {
			return nil, fmt.Errorf("exoscale: only TXT records are supported (got %q)", r.Type)
		}
		if r.Data == "" {
			return nil, fmt.Errorf("exoscale: TXT value must not be empty")
		}
		if r.TTL < 0 || r.TTL%time.Second != 0 {
			return nil, fmt.Errorf("exoscale: TTL must be a non-negative whole number of seconds")
		}
		if strings.HasSuffix(r.Name, ".") {
			return nil, fmt.Errorf("exoscale: record name must be relative to the zone")
		}
		if r.Name == "" {
			r.Name = "@"
		}
		if r.Name != "@" {
			if _, ok := dns.IsDomainName(r.Name); !ok {
				return nil, fmt.Errorf("exoscale: invalid record name")
			}
		}
		out = append(out, libdns.TXT{Name: r.Name, Text: r.Data, TTL: r.TTL})
	}
	return out, nil
}

// wait verifies the final state explicitly: SDK Wait may return an already
// completed operation without checking the requested state.
func (p *Provider) wait(ctx context.Context, op *ego.Operation) error {
	op, err := p.client.Wait(ctx, op, ego.OperationStateSuccess)
	if err != nil {
		return err
	}
	if op == nil || op.State != ego.OperationStateSuccess {
		return errors.New("exoscale: DNS operation did not succeed")
	}
	return nil
}

// AppendRecords adds TXT values without replacing other records at the name.
// A zero TTL uses 60 seconds. On failure, the returned slice contains the
// successfully completed records; batches are not atomic.
func (p *Provider) AppendRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	txts, err := challengeRecords(records)
	if err != nil || len(txts) == 0 {
		return nil, err
	}
	if err = p.lock(ctx); err != nil {
		return nil, err
	}
	defer func() { <-p.gate }()
	id, err := p.zoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	var added []libdns.Record
	for _, r := range txts {
		if r.TTL == 0 {
			r.TTL = 60 * time.Second
		}
		op, err := p.client.CreateDNSDomainRecord(ctx, id, ego.CreateDNSDomainRecordRequest{
			Name: r.Name, Type: ego.CreateDNSDomainRecordRequestTypeTXT, Content: r.Text, Ttl: int64(r.TTL / time.Second),
		})
		if err != nil {
			return added, fmt.Errorf("exoscale: creating TXT record: %w", err)
		}
		if err := p.wait(ctx, op); err != nil {
			return added, fmt.Errorf("exoscale: waiting for TXT creation: %w", err)
		}
		added = append(added, r)
	}
	return added, nil
}

// txtContent accepts the API's raw and DNS presentation forms, including
// multiple quoted strings and escaped characters. Malformed quoting is an error.
func txtContent(content string) (string, error) {
	if !strings.HasPrefix(content, "\"") {
		return content, nil
	}
	r, err := dns.NewRR("example. 60 IN TXT " + content)
	if err != nil {
		return "", err
	}
	// miekg/dns retains presentation escapes in TXT.Txt; libdns expects raw text.
	text := strings.Join(r.(*dns.TXT).Txt, "")
	var decoded strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] != '\\' {
			decoded.WriteByte(text[i])
			continue
		}
		i++
		if i == len(text) {
			return "", errors.New("exoscale: incomplete TXT escape")
		}
		if i+2 < len(text) && text[i] >= '0' && text[i] <= '9' && text[i+1] >= '0' && text[i+1] <= '9' && text[i+2] >= '0' && text[i+2] <= '9' {
			n := int(text[i]-'0')*100 + int(text[i+1]-'0')*10 + int(text[i+2]-'0')
			if n > 255 {
				return "", errors.New("exoscale: invalid TXT decimal escape")
			}
			decoded.WriteByte(byte(n))
			i += 2
		} else {
			decoded.WriteByte(text[i])
		}
	}
	return decoded.String(), nil
}

// DeleteRecords removes only matching TXT values. Empty values and non-TXT
// requests are rejected to prevent broad deletion. TTL zero matches any TTL.
// Missing records are ignored, making ACME cleanup safe to repeat.
func (p *Provider) DeleteRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	txts, err := challengeRecords(records)
	if err != nil || len(txts) == 0 {
		return nil, err
	}
	if err = p.lock(ctx); err != nil {
		return nil, err
	}
	defer func() { <-p.gate }()
	id, err := p.zoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	existing, err := p.client.ListDNSDomainRecords(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("exoscale: listing TXT records: %w", err)
	}
	var deleted []libdns.Record
	removed := make(map[ego.UUID]bool)
	for _, r := range txts {
		for _, candidate := range existing.DNSDomainRecords {
			name := candidate.Name
			if name == "" {
				name = "@"
			}
			if removed[candidate.ID] || !strings.EqualFold(name, r.Name) || string(candidate.Type) != "TXT" {
				continue
			}
			value, err := txtContent(candidate.Content)
			if err != nil {
				return deleted, fmt.Errorf("exoscale: decoding TXT record: %w", err)
			}
			if value != r.Text || (r.TTL != 0 && r.TTL != time.Duration(candidate.Ttl)*time.Second) {
				continue
			}
			op, err := p.client.DeleteDNSDomainRecord(ctx, id, candidate.ID)
			if errors.Is(err, ego.ErrNotFound) {
				removed[candidate.ID] = true
				continue
			}
			if err != nil {
				return deleted, fmt.Errorf("exoscale: deleting TXT record: %w", err)
			}
			if err := p.wait(ctx, op); err != nil {
				return deleted, fmt.Errorf("exoscale: waiting for TXT deletion: %w", err)
			}
			removed[candidate.ID] = true
			deleted = append(deleted, libdns.TXT{Name: r.Name, Text: value, TTL: time.Duration(candidate.Ttl) * time.Second})
		}
	}
	return deleted, nil
}

var (
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
