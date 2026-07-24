package device

import "testing"

func TestStatsVisitorsUnique(t *testing.T) {
	s := NewStats()
	s.NoteVisitor("1.1.1.1")
	s.NoteVisitor("1.1.1.1")
	s.NoteVisitor("2.2.2.2")
	if s.Visitors() != 2 {
		t.Fatalf("visitors=%d want 2", s.Visitors())
	}
}

func TestStatsGameOpen(t *testing.T) {
	s := NewStats()
	s.NoteGameOpen("/games/2048.html")
	s.NoteGameOpen("/games/2048.html")
	s.NoteGameOpen("/games/simon.html")
	s.NoteGameOpen("/index.html") // not a game, ignored
	if s.TotalOpens() != 3 {
		t.Fatalf("total=%d want 3", s.TotalOpens())
	}
	if s.GameHits()["2048"] != 2 || s.GameHits()["simon"] != 1 {
		t.Fatalf("hits=%v", s.GameHits())
	}
	if _, ok := s.GameHits()["index"]; ok {
		t.Fatalf("index should not be counted")
	}
}
