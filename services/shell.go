package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/stevenssparks/webhead/device"
)

// Shell is the device mini-shell. Its identity (prompt, ssid, title, hostname)
// comes from the image manifest so the same binary can present as "spider-verse#"
// or "webhead#".
type Shell struct {
	St       *device.State
	Extended bool
	Prompt   string // e.g. "spider-verse# "
	SSID     string // e.g. "Spider-Verse"
	Title    string // banner title, e.g. "SPIDER-VERSE OS"
	Hostname string // e.g. "spider-verse"
}

func (sh *Shell) promptLine() string {
	if sh.Prompt == "" {
		return "webhead# "
	}
	return sh.Prompt
}

func (sh *Shell) title() string {
	if sh.Title != "" {
		return sh.Title
	}
	return "WEBHEAD OS"
}

func (sh *Shell) ssid() string {
	if sh.SSID != "" {
		return sh.SSID
	}
	return "Webhead"
}

func (sh *Shell) hostname() string {
	if sh.Hostname != "" {
		return sh.Hostname
	}
	return "webhead"
}

func (sh *Shell) Banner() string {
	b := fmt.Sprintf("=== %s shell ===  type 'help'", sh.title())
	if sh.Extended {
		b += "\n[extended mode — emulator-only, beyond the ESP32]"
	}
	return b
}

const helpFaithful = `help              this list
status            board + network summary
clients           # of connected sessions
stats             per-game open counts
log [n]           show last n log lines (default all)
tail              live-stream new web hits (press Enter to stop)
ls [path]         list files in the payload
cat <path>        print a file
rm <path>         delete a file (emulator overlay)
free              memory free
uptime            seconds since boot
wifi              AP details
reboot            restart the emulated device`

const helpExtended = `
pwd               working directory
whoami            current user
echo <text>       print text
uname             system name
clear             clear the screen`

// Run executes one command (except interactive `tail`) and returns its output.
func (sh *Shell) Run(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	arg := ""
	if i := strings.IndexByte(cmd, ' '); i > 0 {
		arg = strings.TrimSpace(cmd[i+1:])
		cmd = cmd[:i]
	}
	st := sh.St
	switch cmd {
	case "help":
		if sh.Extended {
			return helpFaithful + helpExtended
		}
		return helpFaithful
	case "status":
		return fmt.Sprintf(
			"uptime   : %ds\nSSID     : %s   (OPEN)\nhost     : %s\nIP       : 127.0.0.1\nstations : %d connected\nvisitors : %d unique\ngames open: %d total\nheap     : %d KB free (emulated)\npsram    : %d KB free (emulated)\nhttps    : on",
			st.UptimeSec(), sh.ssid(), sh.hostname(), st.Clients(), st.Stats.Visitors(), st.Stats.TotalOpens(), st.HeapKB(), st.PsramKB())
	case "clients":
		return fmt.Sprintf("%d session(s) connected", st.Clients())
	case "stats":
		hits := st.Stats.GameHits()
		if len(hits) == 0 {
			return "no games opened yet"
		}
		names := make([]string, 0, len(hits))
		for k := range hits {
			names = append(names, k)
		}
		sort.Strings(names)
		var b strings.Builder
		b.WriteString("opens  game")
		for _, n := range names {
			fmt.Fprintf(&b, "\n%5d  %s", hits[n], n)
		}
		return b.String()
	case "log":
		n, _ := strconv.Atoi(arg)
		var b strings.Builder
		for i, e := range st.Log.Last(n) {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(e.Text)
		}
		return b.String()
	case "ls":
		p := arg
		if p == "" {
			p = "/"
		}
		items, err := st.FS.List(p)
		if err != nil {
			return "not a directory"
		}
		var b strings.Builder
		for i, fi := range items {
			if i > 0 {
				b.WriteByte('\n')
			}
			name := fi.Name
			if fi.IsDir {
				name += "/"
			}
			fmt.Fprintf(&b, "%8d  %s", fi.Size, name)
		}
		return b.String()
	case "cat":
		b, err := st.FS.Read(arg)
		if err != nil {
			return "no such file"
		}
		return string(b)
	case "rm":
		if err := st.FS.Remove(arg); err != nil {
			return "could not remove"
		}
		return "removed"
	case "free":
		return fmt.Sprintf("heap %d KB / psram %d KB free (emulated)", st.HeapKB(), st.PsramKB())
	case "uptime":
		return fmt.Sprintf("%ds", st.UptimeSec())
	case "wifi":
		return fmt.Sprintf("SSID %s  IP 127.0.0.1  ch 1  stations %d", sh.ssid(), st.Clients())
	case "reboot":
		st.Reboot()
		return "rebooting... (emulated: state cleared)"
	}
	if sh.Extended {
		switch cmd {
		case "pwd":
			return "/"
		case "whoami":
			return "spider"
		case "echo":
			return arg
		case "uname":
			return fmt.Sprintf("%s (emulator) xtensa-esp32s3", sh.title())
		case "clear":
			return "\033[2J\033[H"
		}
	}
	return fmt.Sprintf("unknown: %s  (try 'help')", cmd)
}
