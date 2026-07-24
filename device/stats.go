package device

import (
	"strings"
	"sync"
)

// Stats tracks per-game opens and unique visitors, mirroring the firmware's
// gameHits / visitors / totalOpens.
type Stats struct {
	mu       sync.Mutex
	visitors map[string]struct{}
	games    map[string]uint32
	total    uint32
}

func NewStats() *Stats {
	return &Stats{visitors: map[string]struct{}{}, games: map[string]uint32{}}
}

func (s *Stats) NoteVisitor(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visitors[ip] = struct{}{}
}

// NoteGameOpen increments the counter for a game page — any ".html" under a
// "games/" segment (e.g. /games/2048.html or /arcade/games/2048.html). Other
// URIs are ignored.
func (s *Stats) NoteGameOpen(uri string) {
	if !strings.Contains(uri, "/games/") || !strings.HasSuffix(uri, ".html") {
		return
	}
	name := uri[strings.LastIndexByte(uri, '/')+1 : len(uri)-len(".html")]
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.games[name]++
	s.total++
}

func (s *Stats) Visitors() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.visitors)
}

func (s *Stats) TotalOpens() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(s.total)
}

func (s *Stats) GameHits() map[string]uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]uint32, len(s.games))
	for k, v := range s.games {
		out[k] = v
	}
	return out
}
