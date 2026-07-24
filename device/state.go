package device

import (
	"io/fs"
	"sync"
	"time"
)

type DNSQuery struct {
	Seq    uint32
	Name   string
	Answer string
}

// State is the single shared "device" every service reads and writes, so a hit
// on any web port instantly appears in the SSH tail and the dashboard.
type State struct {
	Log   *LogRing
	Stats *Stats
	FS    *VFS

	mu       sync.Mutex
	boot     time.Time
	dns      []DNSQuery
	dnsSeq   uint32
	sshConns int
	base     fs.FS
}

const logCapacity = 120
const dnsCapacity = 60

func New(base fs.FS) *State {
	return &State{
		Log:   NewLogRing(logCapacity),
		Stats: NewStats(),
		FS:    NewVFS(base),
		boot:  time.Now(),
		base:  base,
	}
}

func (s *State) UptimeSec() int {
	s.mu.Lock()
	b := s.boot
	s.mu.Unlock()
	return int(time.Since(b).Seconds())
}

// NoteHit records a visitor, a possible game open, and a log Entry, returning
// the stored entry.
func (s *State) NoteHit(service, ip, uri string, code int) Entry {
	s.Stats.NoteVisitor(ip)
	s.Stats.NoteGameOpen(uri)
	sec := s.UptimeSec()
	return s.Log.Add(Entry{
		Sec: sec, IP: ip, URI: uri, Service: service, Code: code,
		Text: FormatHit(sec, ip, uri, code),
	})
}

// System adds a free-form "SYS" log line (boot/reboot notices).
func (s *State) System(text string) {
	s.Log.Add(Entry{Sec: s.UptimeSec(), Service: "SYS", Text: text})
}

func (s *State) NoteDNS(name, answer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsSeq++
	s.dns = append(s.dns, DNSQuery{Seq: s.dnsSeq, Name: name, Answer: answer})
	if len(s.dns) > dnsCapacity {
		s.dns = s.dns[len(s.dns)-dnsCapacity:]
	}
}

func (s *State) RecentDNS(n int) []DNSQuery {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n > len(s.dns) {
		n = len(s.dns)
	}
	out := make([]DNSQuery, n)
	copy(out, s.dns[len(s.dns)-n:])
	return out
}

func (s *State) SSHConnect() {
	s.mu.Lock()
	s.sshConns++
	s.mu.Unlock()
}

func (s *State) SSHDisconnect() {
	s.mu.Lock()
	if s.sshConns > 0 {
		s.sshConns--
	}
	s.mu.Unlock()
}

func (s *State) Clients() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sshConns
}

// HeapKB / PsramKB return emulated ESP32-S3-ish free memory: a fixed base with a
// small deterministic wobble so `free`/`status` look alive. Callers label these
// as emulated.
func (s *State) HeapKB() int  { return 210 + int(time.Now().UnixNano()/1e8)%12 }
func (s *State) PsramKB() int { return 7900 + int(time.Now().UnixNano()/1e8)%40 }

// Reboot resets the log, stats, DNS log and uptime, then writes a boot line.
func (s *State) Reboot() {
	s.mu.Lock()
	s.Log = NewLogRing(logCapacity)
	s.Stats = NewStats()
	s.dns = nil
	s.dnsSeq = 0
	s.boot = time.Now()
	s.mu.Unlock()
	s.System("     0s  boot — device up")
}
