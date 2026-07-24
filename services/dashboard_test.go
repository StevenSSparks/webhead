package services

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stevenssparks/webhead/device"
)

func TestDashboardCaptureAPI(t *testing.T) {
	st := device.New(fstest.MapFS{"index.html": {Data: []byte("x")}})
	st.NoteHit("HTTP", "1.1.1.1", "/", 200)
	st.NoteDNS("wififun.net.", "127.0.0.1")
	h := NewDashboard(st, []byte("<html>dash</html>"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/capture", nil))
	if rec.Code != 200 {
		t.Fatalf("capture code=%d", rec.Code)
	}
	var cap []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &cap)
	if len(cap) != 1 {
		t.Fatalf("capture=%v", cap)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/dns", nil))
	var dq []map[string]any
	json.Unmarshal(rec.Body.Bytes(), &dq)
	if len(dq) != 1 || dq[0]["Name"] != "wififun.net." {
		t.Fatalf("dns=%v", dq)
	}
}

func TestDashboardServesHTML(t *testing.T) {
	st := device.New(fstest.MapFS{})
	rec := httptest.NewRecorder()
	NewDashboard(st, []byte("<html>dash</html>")).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 || rec.Body.String() != "<html>dash</html>" {
		t.Fatalf("html code=%d body=%q", rec.Code, rec.Body.String())
	}
}
