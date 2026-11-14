package check

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"testing"
)

// rsaKey returns a base64 public key of the given size. Only the size is
// read, so the modulus does not need to be a real one.
func rsaKey(t *testing.T, bits int) string {
	t.Helper()
	n := new(big.Int).Lsh(big.NewInt(1), uint(bits-1))
	n.SetBit(n, 0, 1)
	der, err := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

// txt writes a TXT record, split into strings of at most 200 characters.
func txt(name, value string) string {
	record := name + " 300 IN TXT"
	for len(value) > 200 {
		record += ` "` + value[:200] + `"`
		value = value[200:]
	}
	return record + ` "` + value + `"`
}

func dkimFor(d *EmailDetails, selector string) DKIMKey {
	for _, k := range d.DKIM {
		if k.Selector == selector {
			return k
		}
	}
	return DKIMKey{}
}

func TestEmailGoodDomain(t *testing.T) {
	ed25519Key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	server := fakeDNS(t,
		"good.test. 300 IN MX 20 mx2.good.test.",
		"good.test. 300 IN MX 10 mx1.good.test.",
		txt("good.test.", "v=spf1 include:_spf.good.test ip4:192.0.2.0/24 ~all"),
		txt("good.test.", "google-site-verification=abc"),
		txt("_spf.good.test.", "v=spf1 include:a.mail.test include:b.mail.test -all"),
		txt("a.mail.test.", "v=spf1 ip4:198.51.100.0/24 -all"),
		txt("b.mail.test.", "v=spf1 a mx -all"),
		txt("_dmarc.good.test.", "v=DMARC1; p=reject; rua=mailto:dmarc@good.test"),
		txt("selector1._domainkey.good.test.", "v=DKIM1; k=rsa; p="+rsaKey(t, 2048)),
		txt("s1._domainkey.good.test.", "v=DKIM1; k=ed25519; p="+ed25519Key),
		txt("custom._domainkey.good.test.", "v=DKIM1; p="+rsaKey(t, 2048)),
		txt("_mta-sts.good.test.", "v=STSv1; id=20261114"),
		txt("_smtp._tls.good.test.", "v=TLSRPTv1; rua=mailto:tls@good.test"),
	)
	r, err := Email(context.Background(), "postmaster@good.test", EmailOptions{Server: server, Selectors: []string{"Custom", "selector1"}})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || r.Target != "good.test" || r.LearnMore != learnEmail {
		t.Fatalf("exit %d, report = %+v", r.ExitCode, r.Sections)
	}
	d := r.Details.(*EmailDetails)
	if len(d.MX) != 2 || d.MX[0].Host != "mx1.good.test" || d.MX[1].Preference != 20 {
		t.Fatalf("mx = %+v", d.MX)
	}
	if d.SPF.Lookups != 5 || d.SPF.All != "~all" || len(d.SPF.Problems) != 0 {
		t.Fatalf("spf = %+v", d.SPF)
	}
	if f := finding(t, r, "spf.lookups"); f.Value != "5 of 10" || f.Status != Pass {
		t.Fatalf("lookups = %+v", f)
	}
	if d.DMARC.Tags["p"] != "reject" || d.MTASTS == "" || d.TLSRPT == "" {
		t.Fatalf("details = %+v", d)
	}
	if k := dkimFor(d, "selector1"); !k.Found || k.KeyType != "rsa" || k.Bits != 2048 {
		t.Fatalf("selector1 = %+v", k)
	}
	if k := dkimFor(d, "s1"); !k.Found || k.KeyType != "ed25519" {
		t.Fatalf("s1 = %+v", k)
	}
	if k := dkimFor(d, "custom"); !k.Found || k.Bits != 2048 {
		t.Fatalf("custom = %+v", k)
	}
	if len(d.DKIM) != len(DKIMSelectors)+1 {
		t.Fatalf("selectors tried = %d", len(d.DKIM))
	}
	if f := finding(t, r, "dkim.missing"); !strings.Contains(f.Value, "default, google") {
		t.Fatalf("missing selectors = %+v", f)
	}
}

func TestEmailProblems(t *testing.T) {
	server := fakeDNS(t,
		txt("open.test.", "v=spf1 include:nospf.test +all"),
		txt("nospf.test.", "hello"),
		txt("_dmarc.open.test.", "v=DMARC1; p=none; pct=50"),
		txt("default._domainkey.open.test.", "v=DKIM1; p="+rsaKey(t, 1024)),
		txt("k1._domainkey.open.test.", "v=DKIM1; p="),
		txt("mail._domainkey.open.test.", "v=DKIM1; p="+rsaKey(t, 512)),
		txt("two.test.", "v=spf1 -all"),
		txt("two.test.", "v=spf1 ip4:192.0.2.1 -all"),
	)
	r, err := Email(context.Background(), "open.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 2 {
		t.Fatalf("exit code = %d", r.ExitCode)
	}
	want := map[string]Status{"mx": Warn, "spf": Pass, "spf.problem": Fail, "spf.all": Fail, "dmarc.p": Warn, "dmarc.pct": Warn, "dmarc.rua": Warn, "mta-sts": Info, "tls-rpt": Info}
	for id, status := range want {
		if f := finding(t, r, id); f.Status != status {
			t.Errorf("%s = %+v, want %s", id, f, status)
		}
	}
	d := r.Details.(*EmailDetails)
	if k := dkimFor(d, "default"); k.Bits != 1024 {
		t.Errorf("default = %+v", k)
	}
	if k := dkimFor(d, "k1"); !k.Revoked {
		t.Errorf("k1 = %+v", k)
	}
	if k := dkimFor(d, "mail"); k.Bits != 512 {
		t.Errorf("mail = %+v", k)
	}
	statuses := map[string]Status{}
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			if f.ID == "dkim" {
				statuses[f.Label] = f.Status
			}
		}
	}
	if statuses["default"] != Warn || statuses["k1"] != Info || statuses["mail"] != Fail {
		t.Errorf("DKIM statuses = %v", statuses)
	}

	r, err = Email(context.Background(), "two.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "spf"); f.Status != Fail || f.Value != "2 SPF records" {
		t.Fatalf("spf = %+v", f)
	}
	if f := finding(t, r, "dmarc"); f.Status != Missing {
		t.Fatalf("dmarc = %+v", f)
	}
	if f := finding(t, r, "dkim.none"); f.Status != Info || !strings.Contains(f.Meaning, "--selector") {
		t.Fatalf("dkim.none = %+v", f)
	}
}

func TestSPFLookupsLoopsAndRedirects(t *testing.T) {
	records := []string{
		txt("loop.test.", "v=spf1 include:loop2.test -all"),
		txt("loop2.test.", "v=spf1 include:loop.test -all"),
		txt("redir.test.", "v=spf1 redirect=_spf.redir.test"),
		txt("_spf.redir.test.", "v=spf1 ip4:192.0.2.1 -all"),
		txt("noall.test.", "v=spf1 ip4:192.0.2.1"),
		txt("ptr.test.", "v=spf1 ptr ?all"),
		txt("deep.test.", "v=spf1 include:la.test -all"),
	}
	// la.test includes lb.test and so on to lk.test: 11 lookups in all.
	letters := "abcdefghijk"
	for i := range len(letters) {
		value := "v=spf1 -all"
		if i+1 < len(letters) {
			value = "v=spf1 include:l" + letters[i+1:i+2] + ".test -all"
		}
		records = append(records, txt("l"+letters[i:i+1]+".test.", value))
	}
	server := fakeDNS(t, records...)

	spfOf := func(domain string) *SPFDetails {
		t.Helper()
		r, err := Email(context.Background(), domain, EmailOptions{Server: server})
		if err != nil {
			t.Fatal(err)
		}
		return &r.Details.(*EmailDetails).SPF
	}
	if spf := spfOf("loop.test"); len(spf.Problems) != 1 || !strings.Contains(spf.Problems[0], "loops back") {
		t.Errorf("loop = %+v", spf)
	}
	if spf := spfOf("redir.test"); spf.All != "-all" || spf.Lookups != 1 {
		t.Errorf("redirect = %+v", spf)
	}
	if spf := spfOf("noall.test"); spf.All != "" || spf.Lookups != 0 {
		t.Errorf("no all = %+v", spf)
	}
	if spf := spfOf("deep.test"); spf.Lookups <= SPFLookupLimit {
		t.Errorf("deep = %+v", spf)
	}

	r, err := Email(context.Background(), "ptr.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "spf.ptr"); f.Status != Warn {
		t.Errorf("ptr = %+v", f)
	}
	if f := finding(t, r, "spf.all"); f.Status != Warn || f.Value != "?all" {
		t.Errorf("?all = %+v", f)
	}
	r, err = Email(context.Background(), "deep.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "spf.lookups"); f.Status != Fail {
		t.Errorf("deep lookups = %+v", f)
	}
	r, err = Email(context.Background(), "redir.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "spf.all"); f.Value != "-all (from redirect=_spf.redir.test)" || f.Status != Pass {
		t.Errorf("redirect all = %+v", f)
	}
}

func TestEmailMissingDomain(t *testing.T) {
	server := fakeDNS(t, txt("other.test.", "x"))
	r, err := Email(context.Background(), "missing.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "mx"); f.Status != Fail || !strings.Contains(f.Value, "does not exist") || r.ExitCode != 2 || hasFinding(r, "spf") {
		t.Fatalf("report = %+v", r.Sections)
	}
	if _, err := Email(context.Background(), "", EmailOptions{Server: server}); err == nil {
		t.Fatal("empty domain gave no error")
	}
}

func TestTagList(t *testing.T) {
	tags := tagList("v=DMARC1; P=reject ;rua=mailto:a@example.com,mailto:b@example.com; p=none")
	if tags["v"] != "DMARC1" || tags["p"] != "reject" || tags["rua"] != "mailto:a@example.com,mailto:b@example.com" {
		t.Fatalf("tags = %v", tags)
	}
}

func TestSPFEvaluationRules(t *testing.T) {
	var records []string
	for i := 1; i <= 11; i++ {
		records = append(records, fmt.Sprintf("manymx.test. 300 IN MX %d mx%d.manymx.test.", i, i))
	}
	records = append(records,
		txt("order.test.", "v=spf1 +all -all"),
		txt("after.test.", "v=spf1 -all include:missing.test"),
		txt("redirall.test.", "v=spf1 redirect=_spf.redirall.test -all"),
		txt("manymx.test.", "v=spf1 mx -all"),
		txt("badip.test.", "v=spf1 ip4:999.999.999.999 ip6:2001:db8::/129 ip4:192.0.2.0/24 ip6:2001:db8::/32 a/33 mx//64 a:x.test/24//64 ip4:2001:db8::1 -all"),
		txt("unknown.test.", "v=spf1 foo:bar -all"),
		txt("macro.test.", "v=spf1 include:%{d}._spf.test exists:%{i}._spf.test -all"),
		txt("macroredir.test.", "v=spf1 redirect=%{d}._spf.test"),
		txt("nested.test.", "v=spf1 include:inner.test -all"),
		txt("inner.test.", "v=spf1 ip4:300.1.1.1 -all"),
	)
	server := fakeDNS(t, records...)
	spfOf := func(domain string) (*SPFDetails, *Report) {
		t.Helper()
		r, err := Email(context.Background(), domain, EmailOptions{Server: server})
		if err != nil {
			t.Fatal(err)
		}
		return &r.Details.(*EmailDetails).SPF, r
	}

	if spf, r := spfOf("order.test"); spf.All != "+all" || finding(t, r, "spf.all").Status != Fail {
		t.Errorf("first all must win: %+v", spf)
	}
	if spf, _ := spfOf("after.test"); spf.All != "-all" || spf.Lookups != 0 || len(spf.Problems) != 0 {
		t.Errorf("terms after all must not be evaluated: %+v", spf)
	}
	if spf, _ := spfOf("redirall.test"); spf.Lookups != 0 || spf.All != "-all" {
		t.Errorf("redirect with all must be ignored: %+v", spf)
	}
	if spf, r := spfOf("manymx.test"); len(spf.Problems) != 1 || !strings.Contains(spf.Problems[0], "11 MX hosts") || r.ExitCode != 2 {
		t.Errorf("mx with 11 hosts = %+v", spf)
	}
	spf, _ := spfOf("badip.test")
	if len(spf.Problems) != 4 {
		t.Errorf("bad addresses: %q", spf.Problems)
	}
	for _, bad := range []string{"ip4:999.999.999.999", "ip6:2001:db8::/129", "a/33", "ip4:2001:db8::1"} {
		if !slices.ContainsFunc(spf.Problems, func(p string) bool { return strings.HasPrefix(p, bad+" ") }) {
			t.Errorf("no problem for %s: %q", bad, spf.Problems)
		}
	}
	if spf, _ := spfOf("unknown.test"); len(spf.Problems) != 1 || !strings.Contains(spf.Problems[0], "not an SPF mechanism") {
		t.Errorf("unknown mechanism: %+v", spf)
	}
	if spf, r := spfOf("macro.test"); len(spf.Macros) != 1 || spf.Macros[0] != "include:%{d}._spf.test" || spf.Lookups != 2 || finding(t, r, "spf.lookups").Status != Warn {
		t.Errorf("macros: %+v", spf)
	}
	if spf, _ := spfOf("macroredir.test"); len(spf.Macros) != 1 || !strings.HasPrefix(spf.Macros[0], "redirect=") || spf.Lookups != 1 {
		t.Errorf("redirect macro: %+v", spf)
	}
	if spf, _ := spfOf("nested.test"); len(spf.Problems) != 1 || !strings.Contains(spf.Problems[0], "in the record of inner.test") {
		t.Errorf("nested syntax error: %+v", spf)
	}
}

func TestDMARCRecordRules(t *testing.T) {
	server := fakeDNS(t,
		txt("case.test.", "v=spf1 -all"),
		txt("_dmarc.case.test.", "v=dmarc1; p=reject; rua=mailto:r@case.test"),
		txt("space.test.", "v=spf1 -all"),
		txt("_dmarc.space.test.", "v = DMARC1; p=reject; rua=mailto:r@space.test"),
		txt("rej.test.", "v=spf1 -all"),
		txt("_dmarc.rej.test.", "v=DMARC1; p=reject; t=y; rua=mailto:r@rej.test"),
		txt("quar.test.", "v=spf1 -all"),
		txt("_dmarc.quar.test.", "v=DMARC1; p=quarantine; t=y; rua=mailto:r@quar.test"),
	)
	email := func(domain string) *Report {
		t.Helper()
		r, err := Email(context.Background(), domain, EmailOptions{Server: server})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if f := finding(t, email("case.test"), "dmarc"); f.Status != Fail || !strings.Contains(f.Meaning, "exactly v=DMARC1") {
		t.Errorf("lower-case version = %+v", f)
	}
	if r := email("space.test"); finding(t, r, "dmarc").Status != Pass || finding(t, r, "dmarc.p").Value != "reject" {
		t.Errorf("spaces around = should be fine: %+v", r.Sections)
	}
	if f := finding(t, email("rej.test"), "dmarc.t"); f.Status != Warn || !strings.Contains(f.Meaning, "quarantine instead of reject") {
		t.Errorf("t=y with reject = %+v", f)
	}
	if f := finding(t, email("quar.test"), "dmarc.t"); !strings.Contains(f.Meaning, "none instead of quarantine") {
		t.Errorf("t=y with quarantine = %+v", f)
	}
}

func TestDKIMAndTransportRecordRules(t *testing.T) {
	key := rsaKey(t, 2048)
	server := fakeDNS(t,
		txt("dk.test.", "v=spf1 -all"),
		txt("default._domainkey.dk.test.", "v=DKIM1; p="),
		txt("v2._domainkey.dk.test.", "v=DKIM2; p="+key),
		txt("nop._domainkey.dk.test.", "v=DKIM1; k=rsa"),
		txt("bang._domainkey.dk.test.", "v=DKIM1; p=!"),
		txt("dupe._domainkey.dk.test.", "v=DKIM1; p="+key+"; p="+key),
		txt("kx._domainkey.dk.test.", "v=DKIM1; k=dsa; p=AAAA"),
		txt("late._domainkey.dk.test.", "k=rsa; v=DKIM1; p="+key),
		txt("junk._domainkey.dk.test.", "v=DKIM1; p=AAAA"),
		txt("_mta-sts.dk.test.", "v=STSv1"),
		txt("_smtp._tls.dk.test.", "v=TLSRPTv1"),
		txt("two.test.", "v=spf1 -all"),
		txt("_mta-sts.two.test.", "v=STSv1; id=a1"),
		txt("_mta-sts.two.test.", "v=STSv1; id=b2"),
		txt("_smtp._tls.two.test.", "v=TLSRPTv1; rua=mailto:a@two.test"),
		txt("_smtp._tls.two.test.", "v=TLSRPTv1; rua=mailto:b@two.test"),
	)
	r, err := Email(context.Background(), "dk.test", EmailOptions{Server: server, Selectors: []string{"v2", "nop", "bang", "dupe", "kx", "late", "junk"}})
	if err != nil {
		t.Fatal(err)
	}
	d := r.Details.(*EmailDetails)
	if k := dkimFor(d, "default"); !k.Revoked || k.Problem != "" {
		t.Errorf("default = %+v", k)
	}
	for _, sel := range []string{"v2", "nop", "bang", "dupe", "kx", "late", "junk"} {
		if k := dkimFor(d, sel); !k.Found || k.Problem == "" {
			t.Errorf("%s = %+v, want a problem", sel, k)
		}
	}
	statuses := map[string]Status{}
	for _, s := range r.Sections {
		for _, f := range s.Findings {
			if f.ID == "dkim" {
				statuses[f.Label] = f.Status
			}
		}
	}
	if statuses["default"] != Info || statuses["nop"] != Fail || statuses["bang"] != Fail {
		t.Errorf("DKIM statuses = %v", statuses)
	}
	if f := finding(t, r, "mta-sts"); f.Status != Fail || !strings.Contains(f.Meaning, "id=") {
		t.Errorf("mta-sts without id = %+v", f)
	}
	if f := finding(t, r, "tls-rpt"); f.Status != Fail || !strings.Contains(f.Meaning, "rua=") {
		t.Errorf("tls-rpt without rua = %+v", f)
	}
	if r.ExitCode != 2 {
		t.Errorf("exit = %d", r.ExitCode)
	}

	r, err = Email(context.Background(), "two.test", EmailOptions{Server: server})
	if err != nil {
		t.Fatal(err)
	}
	if f := finding(t, r, "mta-sts"); f.Status != Fail || f.Value != "2 records" {
		t.Errorf("two MTA-STS records = %+v", f)
	}
	if f := finding(t, r, "tls-rpt"); f.Status != Fail || f.Value != "2 records" {
		t.Errorf("two TLS-RPT records = %+v", f)
	}
}
