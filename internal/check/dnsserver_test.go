package check

import (
	"net"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

// fakeDNS serves the records (in zone file syntax) on 127.0.0.1 over UDP
// and TCP and answers like a recursive resolver: it follows CNAMEs and
// says NXDOMAIN for names with no records. UDP answers that do not fit
// are truncated, so clients must retry over TCP.
func fakeDNS(t *testing.T, records ...string) string {
	t.Helper()
	var rrs []dns.RR
	for _, record := range records {
		rr, err := dns.NewRR(record)
		if err != nil {
			t.Fatalf("bad test record %q: %v", record, err)
		}
		rrs = append(rrs, rr)
	}
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(req)
		q := req.Question[0]
		name := q.Name
		for hops := 0; hops < 8; hops++ {
			exists, alias := false, ""
			for _, rr := range rrs {
				h := rr.Header()
				if !strings.EqualFold(h.Name, name) {
					continue
				}
				exists = true
				if h.Rrtype == q.Qtype {
					m.Answer = append(m.Answer, rr)
				} else if c, ok := rr.(*dns.CNAME); ok && q.Qtype != dns.TypeCNAME {
					m.Answer = append(m.Answer, rr)
					alias = c.Target
				}
			}
			if !exists {
				m.Rcode = dns.RcodeNameError
			}
			if alias == "" {
				break
			}
			name = alias
		}
		if _, udp := w.RemoteAddr().(*net.UDPAddr); udp {
			size := dns.MinMsgSize
			if opt := req.IsEdns0(); opt != nil {
				size = int(opt.UDPSize())
			}
			m.Truncate(size)
		}
		_ = w.WriteMsg(m)
	})

	var udp, tcp *dns.Server
	for attempt := 0; ; attempt++ {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ln, err := net.Listen("tcp", pc.LocalAddr().String())
		if err != nil {
			pc.Close()
			if attempt < 5 {
				continue
			}
			t.Fatal(err)
		}
		udp = &dns.Server{PacketConn: pc, Handler: handler}
		tcp = &dns.Server{Listener: ln, Handler: handler}
		break
	}
	for _, srv := range []*dns.Server{udp, tcp} {
		started := make(chan struct{})
		srv.NotifyStartedFunc = func() { close(started) }
		go func() { _ = srv.ActivateAndServe() }()
		<-started
		t.Cleanup(func() { _ = srv.Shutdown() })
	}
	return udp.PacketConn.LocalAddr().String()
}
