package device

import (
	"testing"
	"testing/fstest"
)

func newTestState() *State {
	return New(fstest.MapFS{
		"index.html":      {Data: []byte("hi")},
		"games/2048.html": {Data: []byte("game")},
	})
}

func TestStateNoteHitFlowsToLogAndStats(t *testing.T) {
	s := newTestState()
	e := s.NoteHit("HTTP", "9.9.9.9", "/games/2048.html", 200)
	if e.Service != "HTTP" || e.Code != 200 {
		t.Fatalf("entry=%+v", e)
	}
	if s.Stats.Visitors() != 1 || s.Stats.TotalOpens() != 1 {
		t.Fatalf("stats not updated: v=%d o=%d", s.Stats.Visitors(), s.Stats.TotalOpens())
	}
	if len(s.Log.Last(0)) != 1 {
		t.Fatalf("log not written")
	}
}

func TestStateDNSLog(t *testing.T) {
	s := newTestState()
	s.NoteDNS("wififun.net", "127.0.0.1")
	s.NoteDNS("captive.apple.com", "127.0.0.1")
	got := s.RecentDNS(10)
	if len(got) != 2 || got[1].Name != "captive.apple.com" || got[1].Answer != "127.0.0.1" {
		t.Fatalf("dns log=%+v", got)
	}
}

func TestStateClients(t *testing.T) {
	s := newTestState()
	s.SSHConnect()
	s.SSHConnect()
	s.SSHDisconnect()
	if s.Clients() != 1 {
		t.Fatalf("clients=%d want 1", s.Clients())
	}
}

func TestStateReboot(t *testing.T) {
	s := newTestState()
	s.NoteHit("HTTP", "1.1.1.1", "/", 200)
	s.Reboot()
	if s.Stats.Visitors() != 0 {
		t.Fatalf("stats not reset")
	}
	last := s.Log.Last(0)
	if len(last) != 1 || last[0].Service != "SYS" {
		t.Fatalf("expected one SYS line after reboot, got %+v", last)
	}
}
