package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stevenssparks/webhead/device"
)

// Version is the webhead release, surfaced by the `version` shell command.
const Version = "0.1.0"

// Shell is the device mini-shell. Its identity (prompt, ssid, title, hostname,
// user) comes from the image manifest so the same binary can present as
// "spider-verse#" or "webhead#".
type Shell struct {
	St       *device.State
	Extended bool
	Prompt   string
	SSID     string
	Title    string
	Hostname string
	User     string
	history  []string // per-session, appended by the SSH transport
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
func (sh *Shell) user() string {
	if sh.User != "" {
		return sh.User
	}
	return "spider"
}

func (sh *Shell) Banner() string {
	b := fmt.Sprintf("=== %s shell ===  type 'help'", sh.title())
	if sh.Extended {
		b += "\n[extended mode — emulator-only, beyond the ESP32]"
	}
	return b
}

// command is one shell command's metadata. transport commands (tail, exit) are
// handled by the SSH loop; they're listed/documented here but have run == nil.
type command struct {
	summary   string
	man       string
	extended  bool
	transport bool
	run       func(sh *Shell, arg string) string
}

// commandOrder controls how `help` lists commands.
var commandOrder = []string{
	"help", "man", "status", "clients", "stats", "log", "tail",
	"ls", "cat", "rm", "free", "uptime", "wifi", "dns", "who", "top",
	"clear", "cls", "reboot", "exit",
	// extended:
	"pwd", "whoami", "id", "echo", "date", "uname", "hostname", "version",
	"history", "about",
}

var commands map[string]*command

func init() {
	commands = map[string]*command{
		"help":    {summary: "this list (or: help <cmd>)", man: "help [command]\n\nWith no argument, list every available command. With a command name,\nshow that command's manual page (same as `man`).", run: (*Shell).cmdHelp},
		"man":     {summary: "show a command's manual", man: "man <command>\n\nPrint the manual page for a command.", run: (*Shell).cmdMan},
		"status":  {summary: "board + network summary", man: "status\n\nShow uptime, SSID, host, IP, connected sessions, unique visitors,\ngames opened, and (emulated) heap/psram.", run: (*Shell).cmdStatus},
		"clients": {summary: "# of connected sessions", man: "clients\n\nHow many SSH sessions are currently connected.", run: (*Shell).cmdClients},
		"stats":   {summary: "per-game open counts", man: "stats\n\nHow many times each game has been opened since boot.", run: (*Shell).cmdStats},
		"log":     {summary: "show last n log lines", man: "log [n]\n\nPrint the last n web-hit log lines (default: all, up to 120).", run: (*Shell).cmdLog},
		"tail":    {summary: "live-stream web hits (Enter to stop)", man: "tail\n\nLive-stream new web hits as they arrive. Press Enter to stop.", transport: true},
		"top":     {summary: "live text dashboard (key to quit)", man: "top\n\nLive-refreshing text dashboard: uptime, sessions, memory, top games,\nand recent web + DNS activity. Press any key to quit.", transport: true},
		"ls":      {summary: "list files in the payload", man: "ls [path]\n\nList files in the served payload (default: /).", run: (*Shell).cmdLs},
		"cat":     {summary: "print a file", man: "cat <path>\n\nPrint the contents of a file from the payload.", run: (*Shell).cmdCat},
		"rm":      {summary: "delete a file (emulator overlay)", man: "rm <path>\n\nHide a file from the served payload. This is an in-memory overlay —\nthe underlying image is never modified.", run: (*Shell).cmdRm},
		"free":    {summary: "memory free", man: "free\n\nShow (emulated) free heap and PSRAM.", run: (*Shell).cmdFree},
		"uptime":  {summary: "seconds since boot", man: "uptime\n\nSeconds since the device booted.", run: (*Shell).cmdUptime},
		"wifi":    {summary: "AP details", man: "wifi\n\nShow the access-point SSID, IP, channel, and connected stations.", run: (*Shell).cmdWifi},
		"dns":     {summary: "recent DNS lookups", man: "dns\n\nShow recent DNS queries and the address they were funneled to.", run: (*Shell).cmdDNS},
		"who":     {summary: "who is connected", man: "who\n\nList currently connected sessions.", run: (*Shell).cmdWho},
		"clear":   {summary: "clear the screen", man: "clear\n\nClear the terminal screen.", run: (*Shell).cmdClear},
		"cls":     {summary: "clear the screen (alias)", man: "cls\n\nAlias for `clear`.", run: (*Shell).cmdClear},
		"reboot":  {summary: "restart the emulated device", man: "reboot\n\nReset uptime, logs, and stats (emulated — the process keeps running).", run: (*Shell).cmdReboot},
		"exit":    {summary: "close this SSH session", man: "exit | logout | quit\n\nDisconnect from the shell. (Ctrl-D also works.)", transport: true},

		// --- extended (emulator-only, beyond the ESP32) ---
		"pwd":      {summary: "working directory", extended: true, man: "pwd\n\nPrint the working directory (always / — the payload root).", run: (*Shell).cmdPwd},
		"whoami":   {summary: "current user", extended: true, man: "whoami\n\nPrint the current user.", run: (*Shell).cmdWhoami},
		"id":       {summary: "user / group ids", extended: true, man: "id\n\nPrint the (emulated) user and group ids.", run: (*Shell).cmdID},
		"echo":     {summary: "print text", extended: true, man: "echo <text>\n\nPrint text back to the terminal.", run: (*Shell).cmdEcho},
		"date":     {summary: "current date/time", extended: true, man: "date\n\nPrint the current date and time.", run: (*Shell).cmdDate},
		"uname":    {summary: "system name", extended: true, man: "uname\n\nPrint the (emulated) system identification.", run: (*Shell).cmdUname},
		"hostname": {summary: "device hostname", extended: true, man: "hostname\n\nPrint the device hostname.", run: (*Shell).cmdHostname},
		"version":  {summary: "webhead version", extended: true, man: "version\n\nPrint the webhead version.", run: (*Shell).cmdVersion},
		"history":  {summary: "command history", extended: true, man: "history\n\nShow the commands entered this session.", run: (*Shell).cmdHistory},
		"about":    {summary: "about this device", extended: true, man: "about\n\nShow a summary splash for this device.", run: (*Shell).cmdAbout},
	}
}

// available reports whether a command can be run/seen in the current mode.
func (sh *Shell) available(c *command) bool { return c != nil && (!c.extended || sh.Extended) }

// Run executes one command (except transport commands like tail/exit, handled by
// the SSH loop) and returns its output.
func (sh *Shell) Run(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return ""
	}
	name := cmd
	arg := ""
	if i := strings.IndexByte(cmd, ' '); i > 0 {
		arg = strings.TrimSpace(cmd[i+1:])
		name = cmd[:i]
	}
	if name == "?" {
		name = "help"
	}
	c, ok := commands[name]
	if !ok || !sh.available(c) {
		return fmt.Sprintf("unknown: %s  (try 'help')", name)
	}
	if c.run == nil {
		return "" // transport-handled; SSH loop intercepts these
	}
	return c.run(sh, arg)
}

func (sh *Shell) cmdHelp(arg string) string {
	if arg != "" {
		return sh.cmdMan(arg)
	}
	var b strings.Builder
	b.WriteString("commands  (type 'man <cmd>' for details):")
	for _, name := range commandOrder {
		c := commands[name]
		if !sh.available(c) {
			continue
		}
		fmt.Fprintf(&b, "\n  %-9s %s", name, c.summary)
	}
	if !sh.Extended {
		b.WriteString("\n\n(more commands available in extended mode)")
	}
	return b.String()
}

func (sh *Shell) cmdMan(arg string) string {
	if arg == "" {
		return "what manual page do you want?  (usage: man <command>)"
	}
	if arg == "?" {
		arg = "help"
	}
	c, ok := commands[arg]
	if !ok {
		return "no manual entry for " + arg
	}
	if !sh.available(c) {
		return "no manual entry for " + arg + " (available in extended mode)"
	}
	return c.man
}

func (sh *Shell) cmdStatus(arg string) string {
	st := sh.St
	return fmt.Sprintf(
		"uptime   : %ds\nSSID     : %s   (OPEN)\nhost     : %s\nIP       : 127.0.0.1\nstations : %d connected\nvisitors : %d unique\ngames open: %d total\nheap     : %d KB free (emulated)\npsram    : %d KB free (emulated)\nhttps    : on",
		st.UptimeSec(), sh.ssid(), sh.hostname(), st.Clients(), st.Stats.Visitors(), st.Stats.TotalOpens(), st.HeapKB(), st.PsramKB())
}

func (sh *Shell) cmdClients(arg string) string {
	return fmt.Sprintf("%d session(s) connected", sh.St.Clients())
}

func (sh *Shell) cmdStats(arg string) string {
	hits := sh.St.Stats.GameHits()
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
}

func (sh *Shell) cmdLog(arg string) string {
	n, _ := strconv.Atoi(arg)
	var b strings.Builder
	for i, e := range sh.St.Log.Last(n) {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Text)
	}
	return b.String()
}

func (sh *Shell) cmdLs(arg string) string {
	p := arg
	if p == "" {
		p = "/"
	}
	items, err := sh.St.FS.List(p)
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
}

func (sh *Shell) cmdCat(arg string) string {
	b, err := sh.St.FS.Read(arg)
	if err != nil {
		return "no such file"
	}
	return string(b)
}

func (sh *Shell) cmdRm(arg string) string {
	if err := sh.St.FS.Remove(arg); err != nil {
		return "could not remove"
	}
	return "removed"
}

func (sh *Shell) cmdFree(arg string) string {
	return fmt.Sprintf("heap %d KB / psram %d KB free (emulated)", sh.St.HeapKB(), sh.St.PsramKB())
}

func (sh *Shell) cmdUptime(arg string) string { return fmt.Sprintf("%ds", sh.St.UptimeSec()) }

func (sh *Shell) cmdWifi(arg string) string {
	return fmt.Sprintf("SSID %s  IP 127.0.0.1  ch 1  stations %d", sh.ssid(), sh.St.Clients())
}

func (sh *Shell) cmdDNS(arg string) string {
	q := sh.St.RecentDNS(20)
	if len(q) == 0 {
		return "no DNS lookups yet"
	}
	var b strings.Builder
	b.WriteString("recent DNS lookups (every name → the board):")
	for _, d := range q {
		fmt.Fprintf(&b, "\n  %-32s → %s", strings.TrimSuffix(d.Name, "."), d.Answer)
	}
	return b.String()
}

func (sh *Shell) cmdWho(arg string) string {
	n := sh.St.Clients()
	return fmt.Sprintf("%s   ssh   (%d session(s) connected)", sh.user(), n)
}

func (sh *Shell) cmdClear(arg string) string { return "\033[2J\033[H" }
func (sh *Shell) cmdReboot(arg string) string {
	sh.St.Reboot()
	return "rebooting... (emulated: state cleared)"
}
func (sh *Shell) cmdPwd(arg string) string    { return "/" }
func (sh *Shell) cmdWhoami(arg string) string { return sh.user() }
func (sh *Shell) cmdID(arg string) string {
	return fmt.Sprintf("uid=1000(%s) gid=1000(%s) groups=1000(%s)", sh.user(), sh.user(), sh.user())
}
func (sh *Shell) cmdEcho(arg string) string { return arg }
func (sh *Shell) cmdDate(arg string) string { return time.Now().Format("Mon Jan  2 15:04:05 MST 2006") }
func (sh *Shell) cmdUname(arg string) string {
	return fmt.Sprintf("%s (emulator) xtensa-esp32s3", sh.title())
}
func (sh *Shell) cmdHostname(arg string) string { return sh.hostname() }
func (sh *Shell) cmdVersion(arg string) string  { return "webhead " + Version }

func (sh *Shell) cmdHistory(arg string) string {
	if len(sh.history) == 0 {
		return "(no history yet)"
	}
	var b strings.Builder
	for i, h := range sh.history {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%4d  %s", i+1, h)
	}
	return b.String()
}

func (sh *Shell) cmdAbout(arg string) string {
	st := sh.St
	return fmt.Sprintf(
		"   \\   /    %s\n"+
			"    \\ /     host   : %s   ssid: %s\n"+
			"  ---🕷---   uptime : %ds\n"+
			"    / \\     memory : %d KB heap / %d KB psram (emulated)\n"+
			"   /   \\    served : %d games opened, %d visitors\n"+
			"           webhead %s",
		sh.title(), sh.hostname(), sh.ssid(), st.UptimeSec(),
		st.HeapKB(), st.PsramKB(), st.Stats.TotalOpens(), st.Stats.Visitors(), Version)
}
