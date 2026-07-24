package services

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	gssh "github.com/gliderlabs/ssh"
	"github.com/stevenssparks/webhead/device"
)

// StartSSH runs the gliderlabs SSH server (blocking; run it in a goroutine).
// shellProto carries the image identity (prompt/ssid/title/hostname/extended);
// each session gets a copy bound to st.
func StartSSH(st *device.State, addr, user, pass string, shellProto Shell) error {
	handler := func(s gssh.Session) {
		st.SSHConnect()
		defer st.SSHDisconnect()

		sh := shellProto
		sh.St = st

		io.WriteString(s, "\n"+sh.Banner()+"\n")
		io.WriteString(s, "\n"+sh.promptLine())

		reader := bufio.NewReader(s)
		for {
			lineText, err := reader.ReadString('\n')
			if err != nil {
				return // client closed
			}
			cmd := strings.TrimRight(lineText, "\r\n")
			if strings.TrimSpace(cmd) == "tail" {
				runTail(s, reader, st)
			} else {
				out := sh.Run(cmd)
				if out != "" {
					io.WriteString(s, out+"\n")
				}
			}
			io.WriteString(s, "\n"+sh.promptLine())
		}
	}

	server := &gssh.Server{
		Addr:    addr,
		Handler: handler,
		PasswordHandler: func(ctx gssh.Context, given string) bool {
			return ctx.User() == user && given == pass
		},
	}
	return server.ListenAndServe()
}

// runTail streams new log lines until the client sends a line (Enter).
func runTail(w io.Writer, reader *bufio.Reader, st *device.State) {
	io.WriteString(w, "[tailing — press Enter to stop]\n")
	last := st.Log.HeadSeq()
	stop := make(chan struct{})
	go func() {
		reader.ReadString('\n')
		close(stop)
	}()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			io.WriteString(w, "[stopped]\n")
			return
		case <-ticker.C:
			for _, e := range st.Log.Since(last) {
				fmt.Fprintln(w, e.Text)
				last = e.Seq
			}
		}
	}
}
