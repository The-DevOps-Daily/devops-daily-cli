package check

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"math/big"
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
	want := map[string]Status{"mx": Warn, "spf": Pass, "spf.include": Fail, "spf.all": Fail, "dmarc.p": Warn, "dmarc.pct": Warn, "dmarc.rua": Warn, "mta-sts": Info, "tls-rpt": Info}
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
	if f := finding(t, r, "dkim.none"); f.Status != Warn {
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
