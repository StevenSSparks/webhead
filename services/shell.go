package services

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stevenssparks/webhead/device"
)

// Version is the webhead release, surfaced by the `version`/`ver` commands and
// the CLI. Override at build time with:
//
//	go build -ldflags "-X github.com/stevenssparks/webhead/services.Version=1.2.3"
var Version = "0.1.0"

// Shell is the device mini-shell. Its identity (prompt, ssid, title, hostname,
// user) comes from the image manifest so the same binary can present as
// "spider-verse#" or "webhead#".
type Shell struct {
	St        *device.State
	Extended  bool
	Prompt    string
	SSID      string
	Title     string
	Hostname  string
	User      string
	Motd      string   // optional login message-of-the-day (from the image manifest)
	DNSAnswer string   // the IP the DNS service funnels every name to
	history   []string // per-session, appended by the SSH transport
	cwd       string   // per-session working directory (extended `cd`)
}

// wd returns the current working directory, defaulting to "/".
func (sh *Shell) wd() string {
	if sh.cwd == "" {
		return "/"
	}
	return sh.cwd
}

// resolve turns a user path argument into an absolute payload path, relative to
// the working directory. "" resolves to the working directory itself.
func (sh *Shell) resolve(arg string) string {
	if arg == "" || arg == "~" {
		if arg == "~" {
			return "/"
		}
		return sh.wd()
	}
	if strings.HasPrefix(arg, "/") {
		return path.Clean(arg)
	}
	return path.Clean(sh.wd() + "/" + arg)
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

// Banner is the login MOTD shown on SSH connect: a unix-style welcome with live
// system stats. Uses \n line endings; the SSH transport converts to \r\n on PTYs.
func (sh *Shell) Banner() string {
	st := sh.St
	line := strings.Repeat("-", 56)
	var b strings.Builder
	fmt.Fprintf(&b, "  ,---.    %s\n", sh.title())
	fmt.Fprintf(&b, " ( o o )   webhead %s\n", Version)
	fmt.Fprintf(&b, "  >|_|<    %s\n", line)
	fmt.Fprintf(&b, "   Welcome, %s. You're on the %s appliance.\n\n", sh.user(), sh.title())
	fmt.Fprintf(&b, "   host    : %-24s  uptime   : %ds\n", sh.hostname(), st.UptimeSec())
	fmt.Fprintf(&b, "   ssid    : %-24s  sessions : %d\n", sh.ssid()+" (OPEN)", st.Clients())
	fmt.Fprintf(&b, "   ip      : %-24s  visitors : %d\n", "127.0.0.1", st.Stats.Visitors())
	fmt.Fprintf(&b, "   https   : %-24s  games    : %d opened\n", "on", st.Stats.TotalOpens())
	fmt.Fprintf(&b, "   heap    : %-24s  psram    : %d KB (emulated)\n",
		fmt.Sprintf("%d KB (emulated)", st.HeapKB()), st.PsramKB())
	fmt.Fprintf(&b, "   %s\n", line)
	if m := strings.TrimRight(sh.Motd, "\n"); m != "" {
		for _, ln := range strings.Split(m, "\n") {
			fmt.Fprintf(&b, "   %s\n", ln)
		}
		fmt.Fprintf(&b, "   %s\n", line)
	}
	b.WriteString("   Type 'help' for commands, 'top' for a live dashboard, 'exit' to leave.\n")
	if sh.Extended {
		b.WriteString("   [extended mode - emulator-only, beyond the ESP32]\n")
	}
	return b.String()
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
	"ls", "cat", "rm", "free", "uptime", "wifi", "dns", "dhcp", "who", "top", "motd",
	"clear", "cls", "reboot", "exit",
	// extended:
	"pwd", "cd", "whoami", "id", "echo", "date", "uname", "hostname", "version",
	"history", "about", "ver",
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
		"dns":     {summary: "DNS server info + stats", man: "dns\n\nShow the DNS server config, total/unique query counts, top names, and\nrecent lookups.", run: (*Shell).cmdDNS},
		"dhcp":    {summary: "DHCP server info + leases", man: "dhcp\n\nShow the DHCP server configuration (gateway, pool, lease time) and the\nnumber of active leases.", run: (*Shell).cmdDHCP},
		"who":     {summary: "who is connected", man: "who\n\nList currently connected sessions.", run: (*Shell).cmdWho},
		"motd":    {summary: "show the message of the day", man: "motd\n\nReprint the login message of the day.", run: (*Shell).cmdMotd},
		"clear":   {summary: "clear the screen", man: "clear\n\nClear the terminal screen.", run: (*Shell).cmdClear},
		"cls":     {summary: "clear the screen (alias)", man: "cls\n\nAlias for `clear`.", run: (*Shell).cmdClear},
		"reboot":  {summary: "restart the emulated device", man: "reboot\n\nReset uptime, logs, and stats (emulated — the process keeps running).", run: (*Shell).cmdReboot},
		"exit":    {summary: "close this SSH session", man: "exit | logout | quit\n\nDisconnect from the shell. (Ctrl-D also works.)", transport: true},

		// --- extended (emulator-only, beyond the ESP32) ---
		"pwd":      {summary: "working directory", extended: true, man: "pwd\n\nPrint the current working directory.", run: (*Shell).cmdPwd},
		"cd":       {summary: "change directory", extended: true, man: "cd [path]\n\nChange the working directory. `cd` with no argument goes to /.", run: (*Shell).cmdCd},
		"whoami":   {summary: "current user", extended: true, man: "whoami\n\nPrint the current user.", run: (*Shell).cmdWhoami},
		"id":       {summary: "user / group ids", extended: true, man: "id\n\nPrint the (emulated) user and group ids.", run: (*Shell).cmdID},
		"echo":     {summary: "print text", extended: true, man: "echo <text>\n\nPrint text back to the terminal.", run: (*Shell).cmdEcho},
		"date":     {summary: "current date/time", extended: true, man: "date\n\nPrint the current date and time.", run: (*Shell).cmdDate},
		"uname":    {summary: "system name", extended: true, man: "uname\n\nPrint the (emulated) system identification.", run: (*Shell).cmdUname},
		"hostname": {summary: "device hostname", extended: true, man: "hostname\n\nPrint the device hostname.", run: (*Shell).cmdHostname},
		"version":  {summary: "webhead version", extended: true, man: "version\n\nPrint the webhead version. (alias: ver)", run: (*Shell).cmdVersion},
		"ver":      {summary: "webhead version (alias)", extended: true, man: "ver\n\nAlias for `version`.", run: (*Shell).cmdVersion},
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
	items, err := sh.St.FS.List(sh.resolve(arg))
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
	if arg == "" {
		return "usage: cat <path>"
	}
	b, err := sh.St.FS.Read(sh.resolve(arg))
	if err != nil {
		return "no such file"
	}
	return string(b)
}

func (sh *Shell) cmdRm(arg string) string {
	if arg == "" {
		return "usage: rm <path>"
	}
	if err := sh.St.FS.Remove(sh.resolve(arg)); err != nil {
		return "could not remove"
	}
	return "removed"
}

func (sh *Shell) cmdCd(arg string) string {
	target := sh.resolve(arg)
	if _, err := sh.St.FS.List(target); err != nil {
		return "cd: not a directory: " + arg
	}
	sh.cwd = target
	return ""
}

func (sh *Shell) cmdFree(arg string) string {
	return fmt.Sprintf("heap %d KB / psram %d KB free (emulated)", sh.St.HeapKB(), sh.St.PsramKB())
}

func (sh *Shell) cmdUptime(arg string) string { return fmt.Sprintf("%ds", sh.St.UptimeSec()) }

func (sh *Shell) cmdWifi(arg string) string {
	return fmt.Sprintf("SSID %s  IP 127.0.0.1  ch 1  stations %d", sh.ssid(), sh.St.Clients())
}

func (sh *Shell) dnsAnswer() string {
	if sh.DNSAnswer != "" {
		return sh.DNSAnswer
	}
	return "127.0.0.1"
}

func (sh *Shell) cmdDNS(arg string) string {
	st := sh.St
	var b strings.Builder
	// config + stats
	fmt.Fprintf(&b, "DNS server (emulated):\n")
	fmt.Fprintf(&b, "  mode    : wildcard — every name resolves to the board\n")
	fmt.Fprintf(&b, "  answer  : %s\n", sh.dnsAnswer())
	fmt.Fprintf(&b, "  queries : %d total, %d unique names\n", st.DNSTotal(), len(st.DNSCounts()))

	// top names
	counts := st.DNSCounts()
	if len(counts) > 0 {
		type nc struct {
			name string
			n    uint32
		}
		var xs []nc
		for k, v := range counts {
			xs = append(xs, nc{strings.TrimSuffix(k, "."), v})
		}
		sort.Slice(xs, func(i, j int) bool {
			if xs[i].n != xs[j].n {
				return xs[i].n > xs[j].n
			}
			return xs[i].name < xs[j].name
		})
		b.WriteString("  top     :")
		for i, x := range xs {
			if i >= 5 {
				break
			}
			fmt.Fprintf(&b, "\n     %5d  %s", x.n, x.name)
		}
	}
	// recent
	if q := st.RecentDNS(8); len(q) > 0 {
		b.WriteString("\n  recent  :")
		for i := len(q) - 1; i >= 0; i-- {
			fmt.Fprintf(&b, "\n     %-30s → %s", strings.TrimSuffix(q[i].Name, "."), q[i].Answer)
		}
	} else {
		b.WriteString("\n  (no lookups yet)")
	}
	return b.String()
}

func (sh *Shell) cmdDHCP(arg string) string {
	// Emulated AP addressing, mirroring the firmware's softAP (apIP 4.3.2.1,
	// MAX_CLIENTS 8).
	active := sh.St.Clients()
	return fmt.Sprintf(
		"DHCP server (emulated):\n"+
			"  gateway : 4.3.2.1\n"+
			"  netmask : 255.255.255.0\n"+
			"  pool    : 4.3.2.2 - 4.3.2.9   (max 8 clients)\n"+
			"  lease   : 7200s\n"+
			"  dns     : 4.3.2.1   (all names → the board)\n"+
			"  active  : %d lease(s)", active)
}

func (sh *Shell) cmdMotd(arg string) string {
	if strings.TrimSpace(sh.Motd) == "" {
		return "(no message of the day)"
	}
	return sh.Motd
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
func (sh *Shell) cmdPwd(arg string) string    { return sh.wd() }
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

// Complete returns a possibly-extended version of line for tab completion, plus
// a list of candidates to display when the completion is ambiguous with no
// further progress. The returned line always has the input as a prefix (tab only
// extends), so the SSH transport can just echo the added characters.
func (sh *Shell) Complete(line string) (string, []string) {
	sp := strings.LastIndex(line, " ")
	if sp < 0 {
		return sh.completeCommand(line)
	}
	return sh.completePath(line[:sp+1], line[sp+1:])
}

func (sh *Shell) completeCommand(prefix string) (string, []string) {
	var m []string
	for _, name := range commandOrder {
		if sh.available(commands[name]) && strings.HasPrefix(name, prefix) {
			m = append(m, name)
		}
	}
	switch {
	case len(m) == 0:
		return prefix, nil
	case len(m) == 1:
		return m[0] + " ", nil
	}
	if cp := longestCommonPrefix(m); cp != prefix {
		return cp, nil
	}
	return prefix, m
}

func (sh *Shell) completePath(head, tok string) (string, []string) {
	dirPart, base := "", tok
	if i := strings.LastIndex(tok, "/"); i >= 0 {
		dirPart, base = tok[:i+1], tok[i+1:]
	}
	items, err := sh.St.FS.List(sh.resolve(dirPart))
	if err != nil {
		return head + tok, nil
	}
	var names []string
	for _, fi := range items {
		if strings.HasPrefix(fi.Name, base) {
			n := fi.Name
			if fi.IsDir {
				n += "/"
			}
			names = append(names, n)
		}
	}
	switch {
	case len(names) == 0:
		return head + tok, nil
	case len(names) == 1:
		full := head + dirPart + names[0]
		if !strings.HasSuffix(names[0], "/") {
			full += " "
		}
		return full, nil
	}
	if cp := longestCommonPrefix(names); cp != base {
		return head + dirPart + cp, nil
	}
	return head + tok, names
}

func longestCommonPrefix(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	p := ss[0]
	for _, s := range ss[1:] {
		for !strings.HasPrefix(s, p) {
			p = p[:len(p)-1]
			if p == "" {
				return ""
			}
		}
	}
	return p
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
