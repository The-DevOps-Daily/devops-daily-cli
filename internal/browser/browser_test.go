package browser

import "testing"

func TestAllowed(t *testing.T) {
	for _, raw := range []string{"https://devops-daily.com/posts/a", "https://go.devops-daily.com/a", "https://github.com/The-DevOps-Daily"} {
		if !Allowed(raw) {
			t.Errorf("Allowed(%q) = false", raw)
		}
	}
	for _, raw := range []string{"http://devops-daily.com", "https://other.example", "javascript:alert(1)", "https://devops-daily.com@evil.example/"} {
		if Allowed(raw) {
			t.Errorf("Allowed(%q) = true", raw)
		}
	}
}
