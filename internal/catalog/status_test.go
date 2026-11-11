package catalog

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
)

// A server (or a proxy on the way) controls the reason phrase of the status
// line. It must never reach an error message, which ends up on a terminal.
func TestSyncErrorLeavesOutServerStatusText(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil || line == "\r\n" {
						break
					}
				}
				_, _ = c.Write([]byte("HTTP/1.1 500 \x1b]2;owned\x07\x1b[31mBad\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"))
			}(conn)
		}
	}()
	store := New(t.TempDir(), "http://"+ln.Addr().String()+"/cli/v1/", "test")
	_, err = store.Sync(context.Background())
	if err == nil {
		t.Fatal("Sync succeeded against a 500")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\x1b\x07") || strings.Contains(msg, "owned") || strings.Contains(msg, "Bad") {
		t.Fatalf("error carries the server's status text: %q", msg)
	}
	if !strings.Contains(msg, "HTTP 500") {
		t.Fatalf("error = %q, want it to name HTTP 500", msg)
	}
}
