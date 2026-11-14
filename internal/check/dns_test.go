package check

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDNSDefaultRecords(t *testing.T) {
	server := fakeDNS(t,
		"example.test. 300 IN A 192.0.2.1",
		"example.test. 300 IN AAAA 2001:db8::1",
		"example.test. 3600 IN MX 10 mail.example.test.",
		"example.test. 86400 IN NS ns1.example.test.",
		`example.test. 300 IN TXT "v=spf1 -all"`,
		`example.test. 3600 IN CAA 0 issue "letsencrypt.org"`,
	)
	r, err := DNS(context.Background(), "Example.test.", DNSOptions{Server: server, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || r.Target != "example.test" || r.LearnMore != learnDNS {
		t.Fatalf("report = %+v", r)
	}
	d := r.Details.(*DNSDetails)
	if len(d.Records) != 6 || len(d.CNAMEChain) != 0 || d.Server != server {
		t.Fatalf("details = %+v", d)
	}
	want := map[string]string{"a": "192.0.2.1", "aaaa": "2001:db8::1", "mx": "10 mail.example.test", "ns": "ns1.example.test", "txt": "v=spf1 -all", "caa": `0 issue "letsencrypt.org"`}
	for id, value := range want {
		if f := finding(t, r, id); f.Value != value || !strings.HasPrefix(f.Label, "TTL ") {
			t.Errorf("%s = %+v, want %q", id, f, value)
		}
	}
	if f := finding(t, r, "mx"); f.Label != "TTL 3600s (1h)" {
		t.Errorf("mx label = %q", f.Label)
	}
	if f := finding(t, r, "cname"); !strings.Contains(f.Value, "not an alias") {
		t.Errorf("cname = %+v", f)
	}
}

func TestDNSCNAMEChainAndDangling(t *testing.T) {
	server := fakeDNS(t,
		"www.example.test. 60 IN CNAME edge.example.net.",
		"edge.example.net. 120 IN CNAME host.example.org.",
		"host.example.org. 30 IN A 192.0.2.10",
		"old.example.test. 60 IN CNAME gone.example.net.",
	)
	r, err := DNS(context.Background(), "www.example.test", DNSOptions{Server: server, Types: []string{"A"}})
	if err != nil {
		t.Fatal(err)
	}
	d := r.Details.(*DNSDetails)
	if len(d.CNAMEChain) != 2 || d.CNAMEChain[0].Value != "edge.example.net" || d.CNAMEChain[1].Value != "host.example.org" {
		t.Fatalf("chain = %+v", d.CNAMEChain)
	}
	if len(d.Records) != 1 || d.Records[0].Name != "host.example.org" || d.Records[0].Value != "192.0.2.10" || r.ExitCode != 0 {
		t.Fatalf("records = %+v, exit %d", d.Records, r.ExitCode)
	}

	r, err = DNS(context.Background(), "old.example.test", DNSOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "nxdomain"); f.Status != Fail || f.Label != "Dangling alias" || !strings.Contains(f.Value, "gone.example.net") || r.ExitCode != 2 {
		t.Fatalf("dangling = %+v, exit %d", f, r.ExitCode)
	}

	r, err = DNS(context.Background(), "nope.example.test", DNSOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "nxdomain"); f.Label != "Not found" || r.ExitCode != 2 {
		t.Fatalf("nxdomain = %+v", f)
	}
}

func TestDNSFollowsCNAMEOnlyAnswers(t *testing.T) {
	server := fakeDNS(t,
		"alias.test. 60 IN CNAME missing.test.",
		"loop1.test. 60 IN CNAME loop2.test.",
		"loop2.test. 60 IN CNAME loop1.test.",
		"www.example.test. 60 IN CNAME edge.example.net.",
		"edge.example.net. 120 IN CNAME host.example.org.",
		"host.example.org. 30 IN A 192.0.2.10",
	)
	r, err := DNS(context.Background(), "alias.test", DNSOptions{Server: server, Types: []string{"CNAME"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "nxdomain"); f.Label != "Dangling alias" || !strings.Contains(f.Value, "missing.test") || r.ExitCode != 2 {
		t.Fatalf("dangling = %+v, exit %d", f, r.ExitCode)
	}
	r, err = DNS(context.Background(), "loop1.test", DNSOptions{Server: server, Types: []string{"CNAME"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "cname.loop"); f.Status != Fail || r.ExitCode != 2 {
		t.Fatalf("loop = %+v", f)
	}
	r, err = DNS(context.Background(), "www.example.test", DNSOptions{Server: server, Types: []string{"CNAME"}})
	if err != nil {
		t.Fatal(err)
	}
	if d := r.Details.(*DNSDetails); len(d.CNAMEChain) != 2 || r.ExitCode != 0 {
		t.Fatalf("chain = %+v, exit %d", d.CNAMEChain, r.ExitCode)
	}
}

func TestDNSInternationalName(t *testing.T) {
	server := fakeDNS(t, "xn--bcher-kva.test. 300 IN A 192.0.2.7")
	r, err := DNS(context.Background(), "Bücher.test", DNSOptions{Server: server, Types: []string{"A"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Target != "xn--bcher-kva.test" || r.Unicode != "bücher.test" || finding(t, r, "a").Value != "192.0.2.7" {
		t.Fatalf("report = %+v", r)
	}
}

func TestDNSTypesAndWarnings(t *testing.T) {
	server := fakeDNS(t,
		"mail-only.test. 300 IN MX 10 mx.example.test.",
		"example.test. 3600 IN SOA ns1.example.test. hostmaster.example.test. 2026111401 7200 3600 1209600 300",
	)
	r, err := DNS(context.Background(), "mail-only.test", DNSOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "address"); f.Status != Warn || r.ExitCode != 1 {
		t.Fatalf("address = %+v, exit %d", f, r.ExitCode)
	}
	if f := finding(t, r, "txt"); f.Status != Info || f.Value != "none" {
		t.Fatalf("txt = %+v", f)
	}

	r, err = DNS(context.Background(), "mail-only.test", DNSOptions{Server: server, Types: []string{"txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "txt"); f.Status != Warn || r.ExitCode != 1 || hasFinding(r, "address") || hasFinding(r, "mx") {
		t.Fatalf("explicit type report = %+v", r.Sections)
	}

	r, err = DNS(context.Background(), "example.test", DNSOptions{Server: server, Types: []string{"soa"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "soa"); !strings.HasPrefix(f.Value, "ns1.example.test hostmaster.example.test serial 2026111401 refresh 2h") {
		t.Fatalf("soa = %+v", f)
	}

	for _, opts := range []DNSOptions{{Server: server, Types: []string{"PTR"}}, {Server: "dns.google"}, {Server: "1.1.1.1:99999"}} {
		if _, err := DNS(context.Background(), "example.test", opts); err == nil {
			t.Errorf("%+v gave no error", opts)
		}
	}
	if _, err := DNS(context.Background(), "192.0.2.1", DNSOptions{Server: server}); err == nil {
		t.Error("an IP address gave no error")
	}
}

func TestDNSLargeAnswerUsesTCP(t *testing.T) {
	long := strings.Repeat("x", 250)
	record := "big.test. 300 IN TXT"
	for i := 0; i < 8; i++ {
		record += ` "` + long + `"`
	}
	server := fakeDNS(t, record)
	r, err := DNS(context.Background(), "big.test", DNSOptions{Server: server, Types: []string{"TXT"}})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "txt"); len(f.Value) != 2000 {
		t.Fatalf("TXT value has %d characters, want 2000: %+v", len(f.Value), r.Sections)
	}
}

func TestDNSTimeout(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	r, err := DNS(context.Background(), "example.test", DNSOptions{Server: pc.LocalAddr().String(), Types: []string{"A"}, Timeout: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "a"); f.Status != Fail || !strings.Contains(f.Value, "within 200ms") || r.ExitCode != 2 {
		t.Fatalf("timeout = %+v", f)
	}
}

func TestServerAddress(t *testing.T) {
	for in, want := range map[string]string{"1.1.1.1": "1.1.1.1:53", "8.8.8.8:5353": "8.8.8.8:5353", "2606:4700:4700::1111": "[2606:4700:4700::1111]:53", "[::1]:53": "[::1]:53"} {
		if got, err := serverAddress(in); err != nil || got != want {
			t.Errorf("serverAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}
