package device

import (
	"fmt"
	"sync"
)

// Entry is one line in the device log ring. Text is the firmware-format
// rendering used by `log`/`tail`; the structured fields feed the dashboard.
type Entry struct {
	Seq     uint32
	Sec     int
	IP      string
	URI     string
	Service string // "HTTP", "HTTPS", "DNS", "SYS"
	Code    int
	Text    string
}

// LogRing is a fixed-capacity ring of Entries with monotonic sequence numbers,
// a direct port of the firmware's 120-line ring buffer.
type LogRing struct {
	mu   sync.Mutex
	buf  []Entry
	head int    // next write index
	n    int    // number stored (<= cap)
	seq  uint32 // last assigned seq
}

func NewLogRing(capacity int) *LogRing {
	if capacity < 1 {
		capacity = 1
	}
	return &LogRing{buf: make([]Entry, capacity)}
}

// Add stores a copy of e, assigning it the next sequence number, and returns
// the stored entry.
func (r *LogRing) Add(e Entry) Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	r.buf[r.head] = e
	r.head = (r.head + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
	return e
}

// ordered returns stored entries oldest-first. Caller holds the lock.
func (r *LogRing) ordered() []Entry {
	out := make([]Entry, r.n)
	start := (r.head - r.n + len(r.buf)) % len(r.buf)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}

// Last returns the last n entries chronologically; n<=0 or n>count returns all.
func (r *LogRing) Last(n int) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	all := r.ordered()
	if n <= 0 || n > len(all) {
		return all
	}
	return all[len(all)-n:]
}

// Since returns all stored entries with Seq > seq, chronological.
func (r *LogRing) Since(seq uint32) []Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Entry
	for _, e := range r.ordered() {
		if e.Seq > seq {
			out = append(out, e)
		}
	}
	return out
}

// HeadSeq returns the highest assigned sequence number.
func (r *LogRing) HeadSeq() uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.seq
}

// FormatHit renders a request in the firmware's exact log-line format.
func FormatHit(sec int, ip, uri string, code int) string {
	return fmt.Sprintf("%6ds  %-15s  %s  (%d)", sec, ip, uri, code)
}
