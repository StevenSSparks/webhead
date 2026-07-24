package services

import (
	"bytes"
	"io"
	"net"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stevenssparks/webhead/device"
	xssh "golang.org/x/crypto/ssh"
)

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
	go StartSSH(st, addr, "spider", "secret", Shell{
		Prompt: "spider-verse# ", SSID: "Spider-Verse", Title: "Spider-Verse OS",
	})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "spider",
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
	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(stdin, "status\n")
	time.Sleep(400 * time.Millisecond)
	stdin.Close()

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("Spider-Verse OS shell")) {
		t.Fatalf("no banner in:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("spider-verse# ")) {
		t.Fatalf("no prompt in:\n%s", out)
	}
	if !bytes.Contains([]byte(out), []byte("SSID     : Spider-Verse")) {
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
	go StartSSH(st, addr, "spider", "secret", Shell{
		Prompt: "spider-verse# ", SSID: "Spider-Verse", Title: "Spider-Verse OS",
	})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "spider",
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
	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	io.WriteString(stdin, "status\r") // carriage return, like a real terminal
	time.Sleep(400 * time.Millisecond)
	stdin.Close()

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("SSID     : Spider-Verse")) {
		t.Fatalf("CR-terminated command did not run (regression: hang):\n%q", out)
	}
}

func TestSSHRejectsWrongPassword(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	st := device.New(fstest.MapFS{})
	go StartSSH(st, addr, "spider", "secret", Shell{})
	waitPort(t, addr)

	cfg := &xssh.ClientConfig{
		User:            "spider",
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
