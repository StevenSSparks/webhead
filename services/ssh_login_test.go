package services

import (
	"bytes"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stevenssparks/webhead/device"
	xssh "golang.org/x/crypto/ssh"
)

// syncBuffer is a goroutine-safe buffer for capturing SSH session output while
// the session writes to it concurrently.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// waitFor polls buf until it contains sub or the deadline passes, returning the
// final contents either way.
func waitFor(buf *syncBuffer, sub string, d time.Duration) string {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), sub) {
			return buf.String()
		}
		time.Sleep(20 * time.Millisecond)
	}
	return buf.String()
}

// TestSSHHostKeyStable proves the persisted host key is identical across server
// restarts (fixes the "host key mismatch / possible MITM" client error).
func TestSSHHostKeyStable(t *testing.T) {
	kp := filepath.Join(t.TempDir(), "ssh_host_key")
	fingerprint := func() string {
		ln, _ := net.Listen("tcp", "127.0.0.1:0")
		addr := ln.Addr().String()
		ln.Close()
		go StartSSH(device.New(fstest.MapFS{}), addr, "u", "p", kp, Shell{})
		waitPort(t, addr)
		var got string
		cfg := &xssh.ClientConfig{
			User: "u",
			Auth: []xssh.AuthMethod{xssh.Password("p")},
			HostKeyCallback: func(_ string, _ net.Addr, key xssh.PublicKey) error {
				got = string(key.Marshal())
				return nil
			},
			Timeout: 3 * time.Second,
		}
		c, err := xssh.Dial("tcp", addr, cfg)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		c.Close()
		return got
	}
	first := fingerprint()
	second := fingerprint()
	if first == "" || first != second {
		t.Fatal("host key changed across restarts — client would see a MITM warning")
	}
}

// TestSSHLoginAndStatus proves a real SSH client can authenticate and drive the
// mini-shell end to end.
func TestSSHLoginAndStatus(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	st := device.New(fstest.MapFS{"index.html": {Data: []byte("x")}})
	st.NoteHit("HTTP", "9.9.9.9", "/", 200)
	go StartSSH(st, addr, "demo", "secret", "", Shell{
		Prompt: "demo# ", SSID: "Demo", Title: "Demo OS",
	})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "demo",
		Auth:            []xssh.AuthMethod{xssh.Password("secret")},
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	client, err := xssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	stdin, _ := sess.StdinPipe()
	buf := &syncBuffer{}
	sess.Stdout = buf
	sess.Stderr = buf
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(stdin, "status\n")
	out := waitFor(buf, "SSID     : Demo", 3*time.Second)
	stdin.Close()

	if !strings.Contains(out, "Demo OS") || !strings.Contains(out, "Welcome,") {
		t.Fatalf("no login banner in:\n%s", out)
	}
	if !strings.Contains(out, "demo# ") {
		t.Fatalf("no prompt in:\n%s", out)
	}
	if !strings.Contains(out, "SSID     : Demo") {
		t.Fatalf("status did not run:\n%s", out)
	}
}

// TestSSHInteractivePTY reproduces a real `ssh` client: it requests a PTY and
// ends the command with a carriage return (\r), not \n. This is the case that
// used to hang.
func TestSSHInteractivePTY(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()

	st := device.New(fstest.MapFS{"index.html": {Data: []byte("x")}})
	go StartSSH(st, addr, "demo", "secret", "", Shell{
		Prompt: "demo# ", SSID: "Demo", Title: "Demo OS",
	})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "demo",
		Auth:            []xssh.AuthMethod{xssh.Password("secret")},
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	client, err := xssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	sess, _ := client.NewSession()
	defer sess.Close()

	modes := xssh.TerminalModes{xssh.ECHO: 0}
	if err := sess.RequestPty("xterm", 40, 80, modes); err != nil {
		t.Fatalf("pty: %v", err)
	}
	stdin, _ := sess.StdinPipe()
	buf := &syncBuffer{}
	sess.Stdout = buf
	sess.Stderr = buf
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(stdin, "status\r") // carriage return, like a real terminal
	out := waitFor(buf, "SSID     : Demo", 3*time.Second)
	stdin.Close()

	if !strings.Contains(out, "SSID     : Demo") {
		t.Fatalf("CR-terminated command did not run (regression: hang):\n%q", out)
	}
}

func TestSSHRejectsWrongPassword(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	st := device.New(fstest.MapFS{})
	go StartSSH(st, addr, "demo", "secret", "", Shell{})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "demo",
		Auth:            []xssh.AuthMethod{xssh.Password("wrong")},
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	if _, err := xssh.Dial("tcp", addr, cfg); err == nil {
		t.Fatal("expected auth failure with wrong password")
	}
}

func waitPort(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server on %s never came up", addr)
}
