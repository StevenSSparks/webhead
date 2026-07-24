package services

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/stevenssparks/webhead/device"
	xssh "golang.org/x/crypto/ssh"
)

// StartSSH runs the gliderlabs SSH server (blocking; run it in a goroutine).
// shellProto carries the image identity (prompt/ssid/title/hostname/extended);
// each session gets a copy bound to st. The session is driven as a raw
// interactive terminal so it works with a real `ssh` client (PTY, CR line
// endings, local echo).
func StartSSH(st *device.State, addr, user, pass, hostKeyPath string, shellProto Shell) error {
	handler := func(s gssh.Session) {
		st.SSHConnect()
		defer st.SSHDisconnect()

		sh := shellProto
		sh.St = st
		_, _, pty := s.Pty()

		writeOut(s, "\r\n"+sh.Banner()+"\r\n\r\n", pty)
		writeOut(s, sh.promptLine(), pty)

		var line []byte
		buf := make([]byte, 1)
		for {
			n, err := s.Read(buf)
			if err != nil {
				return // client closed
			}
			if n == 0 {
				continue
			}
			switch c := buf[0]; c {
			case '\r', '\n':
				writeOut(s, "\r\n", pty)
				cmd := string(line)
				line = line[:0]
				if strings.TrimSpace(cmd) == "tail" {
					runTail(s, st, pty)
				} else if out := sh.Run(cmd); out != "" {
					writeOut(s, out+"\n", pty)
				}
				writeOut(s, sh.promptLine(), pty)
			case 0x7f, 0x08: // backspace / delete
				if len(line) > 0 {
					line = line[:len(line)-1]
					if pty {
						io.WriteString(s, "\b \b")
					}
				}
			case 0x03: // Ctrl-C — abandon the current line
				line = line[:0]
				writeOut(s, "^C\r\n", pty)
				writeOut(s, sh.promptLine(), pty)
			case 0x04: // Ctrl-D — exit on empty line
				if len(line) == 0 {
					writeOut(s, "\r\n", pty)
					return
				}
			default:
				if c >= 0x20 && c < 0x7f {
					line = append(line, c)
					if pty {
						s.Write([]byte{c}) // local echo
					}
				}
			}
		}
	}

	server := &gssh.Server{
		Addr:    addr,
		Handler: handler,
		PasswordHandler: func(ctx gssh.Context, given string) bool {
			return ctx.User() == user && given == pass
		},
	}
	// Use a stable, persisted host key so the client doesn't see a new key on
	// every launch (which reads as a MITM / host-key-mismatch error).
	if signer, err := loadOrCreateHostKey(hostKeyPath); err == nil {
		server.AddHostKey(signer)
	} else if hostKeyPath != "" {
		fmt.Fprintf(os.Stderr,
			"[ssh] WARNING: could not use a stable host key at %s (%v)\n"+
				"[ssh]          falling back to a new key each launch — clients will see a host-key mismatch.\n"+
				"[ssh]          fix: make that path writable (often a root-owned dir from a prior sudo run):\n"+
				"[ssh]          sudo chown -R \"$(id -un)\" %s\n",
			hostKeyPath, err, filepath.Dir(hostKeyPath))
	}
	return server.ListenAndServe()
}

// loadOrCreateHostKey returns a persistent ed25519 SSH host key, generating and
// saving one at path on first use. If path is unusable it returns an error and
// the server falls back to gliderlabs' ephemeral key.
func loadOrCreateHostKey(path string) (gssh.Signer, error) {
	if path == "" {
		return nil, errors.New("no host key path")
	}
	if b, err := os.ReadFile(path); err == nil {
		if s, err := xssh.ParsePrivateKey(b); err == nil {
			return s, nil
		}
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	blk, err := xssh.MarshalPrivateKey(priv, "webhead")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem.EncodeToMemory(blk), 0600); err != nil {
		return nil, err
	}
	return xssh.NewSignerFromKey(priv)
}

// writeOut writes s, converting "\n" to "\r\n" for PTY sessions so multi-line
// output doesn't stair-step.
func writeOut(w io.Writer, s string, pty bool) {
	if pty {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	io.WriteString(w, s)
}

// runTail streams new log lines until the client presses a key (Enter).
func runTail(s gssh.Session, st *device.State, pty bool) {
	writeOut(s, "[tailing — press Enter to stop]\n", pty)
	last := st.Log.HeadSeq()
	stop := make(chan struct{})
	go func() {
		b := make([]byte, 1)
		s.Read(b)
		close(stop)
	}()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			writeOut(s, "[stopped]\n", pty)
			return
		case <-ticker.C:
			for _, e := range st.Log.Since(last) {
				writeOut(s, e.Text+"\n", pty)
				last = e.Seq
			}
		}
	}
}
