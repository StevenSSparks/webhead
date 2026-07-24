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
	sh := &Shell{St: shellState(), SSID: "Spider-Verse"}
	out := sh.Run("status")
	if !strings.Contains(out, "Spider-Verse") || !strings.Contains(out, "OPEN") {
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

func TestShellExtendedOnlyWhenEnabled(t *testing.T) {
	base := &Shell{St: shellState()}
	if !strings.Contains(base.Run("whoami"), "unknown") {
		t.Fatal("whoami should be unknown in faithful mode")
	}
	ext := &Shell{St: shellState(), Extended: true}
	if strings.TrimSpace(ext.Run("whoami")) != "spider" {
		t.Fatalf("whoami=%q", ext.Run("whoami"))
	}
}
