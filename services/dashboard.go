package services

import (
	"encoding/json"
	"net/http"

	"github.com/stevenssparks/roost/device"
)

// NewDashboard serves the live console. indexHTML is the embedded dashboard page
// (passed in so this package stays free of embed wiring).
func NewDashboard(st *device.State, indexHTML []byte) http.Handler {
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("/api/capture", func(w http.ResponseWriter, r *http.Request) {
		all := st.Log.Last(60)
		rev := make([]device.Entry, len(all))
		for i, e := range all {
			rev[len(all)-1-i] = e
		}
		writeJSON(w, rev)
	})
	mux.HandleFunc("/api/dns", func(w http.ResponseWriter, r *http.Request) {
		all := st.RecentDNS(60)
		rev := make([]device.DNSQuery, len(all))
		for i, d := range all {
			rev[len(all)-1-i] = d
		}
		writeJSON(w, rev)
	})
	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"visitors":   st.Stats.Visitors(),
			"totalOpens": st.Stats.TotalOpens(),
			"uptimeSec":  st.UptimeSec(),
			"clients":    st.Clients(),
			"games":      st.Stats.GameHits(),
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	return mux
}
