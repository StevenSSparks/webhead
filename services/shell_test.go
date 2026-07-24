package services

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stevenssparks/webhead/device"
)

func shellState() *device.State {
	st := device.New(fstest.MapFS{
		"index.html":      {Data: []byte("<h1>arcade</h1>")},
		"games/2048.html": {Data: []byte("2048")},
	})
	st.NoteHit("HTTP", "1.2.3.4", "/games/2048.html", 200)
	return st
}

func TestShellHelpListsCommands(t *testing.T) {
	sh := &Shell{St: shellState()}
	out := sh.Run("help")
	for _, c := range []string{"status", "tail", "ls", "cat", "reboot"} {
		if !strings.Contains(out, c) {
			t.Fatalf("help missing %q:\n%s", c, out)
		}
	}
}

func TestShellStatusShowsSSID(t *testing.T) {
	sh := &Shell{St: shellState(), SSID: "Demo"}
	out := sh.Run("status")
	if !strings.Contains(out, "Demo") || !strings.Contains(out, "OPEN") {
		t.Fatalf("status:\n%s", out)
	}
}

func TestShellLsAndCat(t *testing.T) {
	sh := &Shell{St: shellState()}
	if !strings.Contains(sh.Run("ls /"), "index.html") {
		t.Fatalf("ls / missing index.html")
	}
	if sh.Run("cat /index.html") != "<h1>arcade</h1>" {
		t.Fatalf("cat wrong: %q", sh.Run("cat /index.html"))
	}
	if !strings.Contains(sh.Run("cat /nope"), "no such file") {
		t.Fatal("cat missing-file message wrong")
	}
}

func TestShellStatsCountsGame(t *testing.T) {
	sh := &Shell{St: shellState()}
	if !strings.Contains(sh.Run("stats"), "2048") {
		t.Fatalf("stats:\n%s", sh.Run("stats"))
	}
}

func TestShellRmRemoves(t *testing.T) {
	sh := &Shell{St: shellState()}
	if !strings.Contains(sh.Run("rm /games/2048.html"), "removed") {
		t.Fatal("rm should report removed")
	}
	if sh.St.FS.Exists("/games/2048.html") {
		t.Fatal("file should be gone from view")
	}
}

func TestShellUnknown(t *testing.T) {
	sh := &Shell{St: shellState()}
	if !strings.Contains(sh.Run("frobnicate"), "unknown") {
		t.Fatal("unknown command message wrong")
	}
}

func TestShellHelpListsSessionAndTopCommands(t *testing.T) {
	sh := &Shell{St: shellState()}
	out := sh.Run("help")
	for _, c := range []string{"man", "top", "exit", "clear", "who", "dns"} {
		if !strings.Contains(out, c) {
			t.Fatalf("help missing %q:\n%s", c, out)
		}
	}
}

func TestShellManPages(t *testing.T) {
	sh := &Shell{St: shellState()}
	if !strings.Contains(sh.Run("man status"), "uptime") {
		t.Fatalf("man status wrong: %q", sh.Run("man status"))
	}
	// ? is an alias for help
	if sh.Run("?") != sh.Run("help") {
		t.Fatal("? should alias help")
	}
	// man for an extended cmd in faithful mode explains it needs extended mode
	if !strings.Contains(sh.Run("man whoami"), "extended") {
		t.Fatalf("man whoami (faithful) should mention extended: %q", sh.Run("man whoami"))
	}
}

func TestShellClearAndAliasAlwaysAvailable(t *testing.T) {
	sh := &Shell{St: shellState()} // faithful mode
	if sh.Run("clear") == "" || !strings.Contains(sh.Run("clear"), "\033[2J") {
		t.Fatal("clear should emit an ANSI clear even in faithful mode")
	}
	if sh.Run("cls") != sh.Run("clear") {
		t.Fatal("cls should equal clear")
	}
}

func TestShellExtendedExtras(t *testing.T) {
	ext := &Shell{St: shellState(), Extended: true, User: "demo", Hostname: "demo"}
	if ext.Run("version") != "webhead "+Version {
		t.Fatalf("version: %q", ext.Run("version"))
	}
	if ext.Run("hostname") != "demo" {
		t.Fatalf("hostname: %q", ext.Run("hostname"))
	}
	if !strings.Contains(ext.Run("id"), "demo") {
		t.Fatalf("id: %q", ext.Run("id"))
	}
	if ext.Run("echo hello there") != "hello there" {
		t.Fatalf("echo: %q", ext.Run("echo hello there"))
	}
	// date should be non-empty
	if ext.Run("date") == "" {
		t.Fatal("date empty")
	}
}

func TestShellCd(t *testing.T) {
	st := device.New(fstest.MapFS{
		"index.html":             {Data: []byte("x")},
		"arcade/games/2048.html": {Data: []byte("g")},
	})
	sh := &Shell{St: st, Extended: true}
	if sh.Run("pwd") != "/" {
		t.Fatalf("pwd default: %q", sh.Run("pwd"))
	}
	if out := sh.Run("cd arcade"); out != "" {
		t.Fatalf("cd arcade: %q", out)
	}
	if sh.Run("pwd") != "/arcade" {
		t.Fatalf("pwd after cd: %q", sh.Run("pwd"))
	}
	if !strings.Contains(sh.Run("ls"), "games") {
		t.Fatalf("ls in /arcade: %q", sh.Run("ls"))
	}
	if !strings.Contains(sh.Run("cd nope"), "not a directory") {
		t.Fatal("cd into bad dir should error")
	}
	sh.Run("cd /")
	if sh.Run("pwd") != "/" {
		t.Fatal("cd / should return to root")
	}
}

func TestShellComplete(t *testing.T) {
	st := device.New(fstest.MapFS{
		"index.html":             {Data: []byte("x")},
		"arcade/games/2048.html": {Data: []byte("g")},
	})
	sh := &Shell{St: st, Extended: true}
	if nl, _ := sh.Complete("vers"); nl != "version " {
		t.Fatalf("command complete: %q", nl)
	}
	if nl, _ := sh.Complete("cat inde"); nl != "cat index.html " {
		t.Fatalf("file complete: %q", nl)
	}
	if nl, _ := sh.Complete("ls arca"); nl != "ls arcade/" {
		t.Fatalf("dir complete: %q", nl)
	}
}

func TestShellExtendedOnlyWhenEnabled(t *testing.T) {
	base := &Shell{St: shellState()}
	if !strings.Contains(base.Run("whoami"), "unknown") {
		t.Fatal("whoami should be unknown in faithful mode")
	}
	ext := &Shell{St: shellState(), Extended: true}
	if strings.TrimSpace(ext.Run("whoami")) != "webhead" {
		t.Fatalf("whoami=%q", ext.Run("whoami"))
	}
}
