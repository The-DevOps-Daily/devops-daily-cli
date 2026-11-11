package catalog

import (
	"testing"
	"time"
)

func TestValidBaseURL(t *testing.T) {
	for _, raw := range []string{"https://devops-daily.com/cli/v1/", "http://localhost:8080/", "http://127.0.0.1:8080/", "http://[::1]:8080/"} {
		if !ValidBaseURL(raw) {
			t.Errorf("ValidBaseURL(%q) = false", raw)
		}
	}
	for _, raw := range []string{"http://example.com/", "ftp://localhost/", "https://devops-daily.com@evil.example/"} {
		if ValidBaseURL(raw) {
			t.Errorf("ValidBaseURL(%q) = true", raw)
		}
	}
}

func TestSponsorOnRotatesUTC(t *testing.T) {
	sponsors := []Sponsor{{Name: "one"}, {Name: "two"}}
	first, _ := SponsorOn(sponsors, time.Date(2026, 1, 1, 23, 0, 0, 0, time.FixedZone("west", -5*3600)))
	second, _ := SponsorOn(sponsors, time.Date(2026, 1, 2, 23, 0, 0, 0, time.FixedZone("west", -5*3600)))
	if first.Name == second.Name {
		t.Fatal("sponsor did not rotate by UTC day")
	}
}
