package device

import "testing"

func TestLogRingAddAndLast(t *testing.T) {
	r := NewLogRing(3)
	r.Add(Entry{Text: "a"})
	r.Add(Entry{Text: "b"})
	got := r.Last(0)
	if len(got) != 2 || got[0].Text != "a" || got[1].Text != "b" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("seq not assigned: %+v", got)
	}
}

func TestLogRingWraps(t *testing.T) {
	r := NewLogRing(3)
	for _, s := range []string{"a", "b", "c", "d"} {
		r.Add(Entry{Text: s})
	}
	got := r.Last(0)
	if len(got) != 3 || got[0].Text != "b" || got[2].Text != "d" {
		t.Fatalf("wrap wrong: %+v", got)
	}
}

func TestLogRingSince(t *testing.T) {
	r := NewLogRing(10)
	for _, s := range []string{"a", "b", "c"} {
		r.Add(Entry{Text: s})
	}
	got := r.Since(1)
	if len(got) != 2 || got[0].Text != "b" || got[1].Text != "c" {
		t.Fatalf("since wrong: %+v", got)
	}
	if len(r.Since(3)) != 0 {
		t.Fatalf("since head should be empty")
	}
}

func TestFormatHit(t *testing.T) {
	got := FormatHit(42, "127.0.0.1", "/games/2048.html", 200)
	want := "    42s  127.0.0.1        /games/2048.html  (200)"
	if got != want {
		t.Fatalf("format:\n got %q\nwant %q", got, want)
	}
}
