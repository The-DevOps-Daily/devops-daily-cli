// Package browser opens approved DevOps Daily links in a local browser.
package browser

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
)

// Allowed reports whether url is one of the links this CLI may open.
func Allowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return false
	}
	return u.Hostname() == "devops-daily.com" || u.Hostname() == "go.devops-daily.com" || u.Hostname() == "github.com"
}

// Open starts a browser process without waiting for it.
func Open(raw string) error {
	if !Allowed(raw) {
		return fmt.Errorf("refusing to open URL: %s", raw)
	}
	if os.Getenv("SSH_CONNECTION") != "" {
		return fmt.Errorf("no local browser in an SSH session")
	}
	if runtime.GOOS == "linux" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return fmt.Errorf("no graphical display is available")
	}
	var name string
	var args []string
	if b := os.Getenv("BROWSER"); b != "" {
		name, args = b, []string{raw}
	} else {
		switch runtime.GOOS {
		case "darwin":
			name, args = "open", []string{raw}
		case "windows":
			name, args = "rundll32", []string{"url.dll,FileProtocolHandler", raw}
		default:
			name, args = "xdg-open", []string{raw}
		}
	}
	return exec.Command(name, args...).Start()
}
