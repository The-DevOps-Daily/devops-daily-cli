// Command ssh-site serves the DevOps Daily terminal app over SSH.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/The-DevOps-Daily/devops-daily-cli/internal/content"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/feed"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/limits"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/server"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/sponsor"
	"github.com/The-DevOps-Daily/devops-daily-cli/internal/tui"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ssh-site:", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// systemd sets STATE_DIRECTORY when the unit uses StateDirectory=.
func defaultStateDir() string {
	if v := os.Getenv("STATE_DIRECTORY"); v != "" {
		return strings.Split(v, ":")[0]
	}
	return "state"
}

func run() error {
	var (
		listen      = flag.String("listen", env("SSH_SITE_LISTEN", "127.0.0.1:2222"), "address to listen on, host:port")
		stateDir    = flag.String("state-dir", defaultStateDir(), "directory for the host key and the feed cache")
		sponsorFile = flag.String("sponsor-file", env("SSH_SITE_SPONSOR_FILE", "/etc/ssh-site/sponsor.conf"), "sponsor slot config")
		feedURL     = flag.String("feed-url", env("SSH_SITE_FEED_URL", "https://devops-daily.com/feed.xml"), "RSS feed for Latest posts")
		feedEvery   = flag.Duration("feed-interval", 30*time.Minute, "feed refresh period")
		rate        = flag.Int("rate-per-ip", limits.DefaultRatePerIP, "new connections per address per minute")
		maxTotal    = flag.Int("max-sessions", limits.DefaultMaxTotal, "concurrent connections in total")
		maxPerIP    = flag.Int("max-per-ip", limits.DefaultMaxPerIP, "concurrent connections per address")
		idle        = flag.Duration("idle-timeout", 10*time.Minute, "end a session after this long without a key press")
		maxSession  = flag.Duration("max-session", 30*time.Minute, "longest session")
		printFP     = flag.Bool("fingerprint", false, "print the host key fingerprint and exit")
		showVersion = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println("ssh-site", version)
		return nil
	}
	hostKey := filepath.Join(*stateDir, "host_ed25519")
	if *printFP {
		signer, err := server.LoadHostKey(hostKey)
		if err != nil {
			return err
		}
		fmt.Println(server.Fingerprint(signer))
		return nil
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 && os.Getenv("JOURNAL_STREAM") != "" {
				return slog.Attr{} // journald adds its own timestamp
			}
			return a
		},
	}))

	snap, err := content.Load()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*stateDir, 0o700); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	posts := feed.New(feed.Options{
		URL:      *feedURL,
		Interval: *feedEvery,
		File:     filepath.Join(*stateDir, "feed.json"),
		Logger:   log,
	})
	if err := posts.Load(); err != nil {
		log.Warn("feed cache not loaded", "err", err)
	}
	go posts.Run(ctx)

	spons := sponsor.NewSource(*sponsorFile, 10*time.Second)

	srv, err := server.New(server.Config{
		HostKeyPath: hostKey,
		Limits: limits.Config{
			RatePerIP: *rate,
			Window:    time.Minute,
			MaxTotal:  *maxTotal,
			MaxPerIP:  *maxPerIP,
		},
		IdleTimeout: *idle,
		MaxSession:  *maxSession,
		Logger:      log,
		App: func(r *lipgloss.Renderer, onQuit func(string)) tea.Model {
			return tui.New(r, tui.Deps{
				Posts:       posts.Posts,
				Content:     snap,
				Sponsor:     spons.Get,
				IdleTimeout: *idle,
				MaxSession:  *maxSession,
				OnQuit:      onQuit,
			})
		},
	})
	if err != nil {
		return err
	}

	log.Info("starting", "version", version, "listen", *listen, "host_key", srv.Fingerprint(),
		"questions", len(snap.Questions), "incidents", len(snap.Incidents), "snapshot", snap.Generated)
	return srv.ListenAndServe(ctx, *listen)
}
