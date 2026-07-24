package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stevenssparks/webhead/device"
)

func testState() *device.State {
	return device.New(fstest.MapFS{
		"index.html":      {Data: []byte("<h1>arcade</h1>")},
		"admin.html":      {Data: []byte("<h1>admin</h1>")},
		"games/2048.html": {Data: []byte("2048")},
	})
}

func do(h http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "5.6.7.8:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRootServesArcade(t *testing.T) {
	rec := do(NewHandler(testState(), "HTTP"), "GET", "/")
	if rec.Code != 200 || rec.Body.String() != "<h1>arcade</h1>" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestCaptiveProbeRedirects(t *testing.T) {
	h := NewHandler(testState(), "HTTP")
	for _, p := range []string{"/generate_204", "/hotspot-detect.html", "/ncsi.txt"} {
		if rec := do(h, "GET", p); rec.Code != 302 {
			t.Fatalf("%s code=%d want 302", p, rec.Code)
		}
	}
}

func TestUnknownRedirects(t *testing.T) {
	if rec := do(NewHandler(testState(), "HTTP"), "GET", "/nope/whatever"); rec.Code != 302 {
		t.Fatalf("code=%d want 302", rec.Code)
	}
}

func TestGameServed(t *testing.T) {
	rec := do(NewHandler(testState(), "HTTP"), "GET", "/games/2048.html")
	if rec.Code != 200 || rec.Body.String() != "2048" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestStatsJSON(t *testing.T) {
	st := testState()
	h := NewHandler(st, "HTTP")
	do(h, "GET", "/games/2048.html")
	rec := do(h, "GET", "/api/stats")
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing no-store")
	}
	var v map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v["totalOpens"].(float64) != 1 {
		t.Fatalf("stats=%v", v)
	}
}
