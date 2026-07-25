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
	"sort"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/stevenssparks/roost/device"
	xssh "golang.org/x/crypto/ssh"
)

const (
	maxHistory = 10  // commands kept per session for `history`
	maxLineLen = 256 // max input line length (chars)
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
				trimmed := strings.TrimSpace(cmd)
				if trimmed != "" {
					sh.history = append(sh.history, trimmed)
					if len(sh.history) > maxHistory {
						sh.history = sh.history[len(sh.history)-maxHistory:]
					}
				}
				switch trimmed {
				case "tail":
					runTail(s, st, pty)
				case "top":
					runTop(s, &sh, st, pty)
				case "exit", "logout", "quit":
					writeOut(s, "logout\r\n", pty)
					return
				default:
					if out := sh.Run(cmd); out != "" {
						writeOut(s, out+"\n", pty)
					}
				}
				writeOut(s, sh.promptLine(), pty)
			case 0x7f, 0x08: // backspace / delete
				if len(line) > 0 {
					line = line[:len(line)-1]
					if pty {
						io.WriteString(s, "\b \b")
					}
				}
			case '\t': // tab completion (commands and file paths)
				cur := string(line)
				newLine, options := sh.Complete(cur)
				if newLine != cur {
					extra := newLine[len(cur):]
					line = []byte(newLine)
					if pty {
						io.WriteString(s, extra)
					}
				}
				if len(options) > 0 {
					writeOut(s, "\r\n"+strings.Join(options, "   ")+"\r\n", pty)
					writeOut(s, sh.promptLine(), pty)
					if pty {
						s.Write(line)
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
				if c >= 0x20 && c < 0x7f && len(line) < maxLineLen {
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
	blk, err := xssh.MarshalPrivateKey(priv, "roost")
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

// runTop renders a live text dashboard until the client presses any key.
func runTop(s gssh.Session, sh *Shell, st *device.State, pty bool) {
	stop := make(chan struct{})
	go func() {
		b := make([]byte, 1)
		s.Read(b)
		close(stop)
	}()

	bar := func(freeKB, capKB int) string {
		used := capKB - freeKB
		if used < 0 {
			used = 0
		}
		n := 0
		if capKB > 0 {
			n = used * 16 / capKB
		}
		if n > 16 {
			n = 16
		}
		return "[" + strings.Repeat("#", n) + strings.Repeat("-", 16-n) + "]"
	}

	draw := func() {
		var b strings.Builder
		if pty {
			b.WriteString("\033[2J\033[H") // clear + home
		}
		line := strings.Repeat("-", 52)
		fmt.Fprintf(&b, "%s - top\r\n%s\r\n", sh.title(), line)
		fmt.Fprintf(&b, " uptime %-6ds  sessions %-3d  visitors %-4d  games %-4d\r\n",
			st.UptimeSec(), st.Clients(), st.Stats.Visitors(), st.Stats.TotalOpens())
		fmt.Fprintf(&b, " heap  %s %d KB\r\n", bar(st.HeapKB(), 320), st.HeapKB())
		fmt.Fprintf(&b, " psram %s %d KB   (emulated)\r\n%s\r\n", bar(st.PsramKB(), 8192), st.PsramKB(), line)

		hits := st.Stats.GameHits()
		if len(hits) > 0 {
			type gh struct {
				name string
				n    uint32
			}
			var gs []gh
			for k, v := range hits {
				gs = append(gs, gh{k, v})
			}
			sort.Slice(gs, func(i, j int) bool { return gs[i].n > gs[j].n })
			b.WriteString(" top games:\r\n")
			for i, g := range gs {
				if i >= 5 {
					break
				}
				fmt.Fprintf(&b, "   %5d  %s\r\n", g.n, g.name)
			}
			b.WriteString(line + "\r\n")
		}

		b.WriteString(" recent hits:\r\n")
		hitsLog := st.Log.Last(6)
		if len(hitsLog) == 0 {
			b.WriteString("   (none yet)\r\n")
		}
		for i := len(hitsLog) - 1; i >= 0; i-- {
			t := hitsLog[i].Text
			if len(t) > 50 {
				t = t[:50]
			}
			fmt.Fprintf(&b, "   %s\r\n", t)
		}

		if dq := st.RecentDNS(3); len(dq) > 0 {
			b.WriteString(" recent dns:\r\n")
			for i := len(dq) - 1; i >= 0; i-- {
				fmt.Fprintf(&b, "   %-30s -> %s\r\n", strings.TrimSuffix(dq[i].Name, "."), dq[i].Answer)
			}
		}
		fmt.Fprintf(&b, "%s\r\n press any key to quit\r\n", line)
		io.WriteString(s, b.String())
	}

	draw()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			writeOut(s, "\r\n", pty)
			return
		case <-ticker.C:
			draw()
		}
	}
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
