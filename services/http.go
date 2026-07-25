// Package services implements Roost's emulated network services (HTTP, HTTPS,
// DNS, SSH, dashboard) over a shared device.State.
package services

import (
	"encoding/json"
	"net"
	"net/http"
	"path"
	"strings"

	"github.com/stevenssparks/roost/device"
)

var captiveProbes = map[string]bool{
	"/generate_204":        true,
	"/gen_204":             true,
	"/hotspot-detect.html": true,
	"/ncsi.txt":            true,
	"/connecttest.txt":     true,
	"/canonical.html":      true,
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func contentType(p string) string {
	switch {
	case strings.HasSuffix(p, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(p, ".css"):
		return "text/css"
	case strings.HasSuffix(p, ".js"):
		return "application/javascript"
	case strings.HasSuffix(p, ".json"):
		return "application/json"
	case strings.HasSuffix(p, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(p, ".png"):
		return "image/png"
	default:
		return "text/plain; charset=utf-8"
	}
}

// NewHandler builds the shared router used by both HTTP and HTTPS. service is
// "HTTP" or "HTTPS" and tags every logged hit.
func NewHandler(st *device.State, service string) http.Handler {
	mux := http.NewServeMux()

	redirect := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
		st.NoteHit(service, clientIP(r), r.URL.Path, http.StatusFound)
	}

	serve := func(w http.ResponseWriter, r *http.Request, fsPath string) bool {
		b, err := st.FS.Read(fsPath)
		if err != nil {
			return false
		}
		w.Header().Set("Content-Type", contentType(fsPath))
		w.WriteHeader(http.StatusOK)
		w.Write(b)
		st.NoteHit(service, clientIP(r), r.URL.Path, http.StatusOK)
		return true
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case captiveProbes[p]:
			redirect(w, r)
		case p == "/":
			if !serve(w, r, "/index.html") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write([]byte("<h1>Roost</h1><p>image payload missing</p>"))
				st.NoteHit(service, clientIP(r), "/", http.StatusOK)
			}
		case p == "/spidey-admin":
			if !serve(w, r, "/admin.html") {
				http.Error(w, "admin.html missing", http.StatusNotFound)
				st.NoteHit(service, clientIP(r), p, http.StatusNotFound)
			}
		case p == "/api/stats":
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"visitors":   st.Stats.Visitors(),
				"totalOpens": st.Stats.TotalOpens(),
				"uptimeSec":  st.UptimeSec(),
				"games":      st.Stats.GameHits(),
			})
			st.NoteHit(service, clientIP(r), "/api/stats", http.StatusOK)
		default:
			// serve the requested path, resolving directory-style requests
			// (e.g. /heroes/, /arcade/) to their index.html
			clean := path.Clean(p)
			switch {
			case strings.HasSuffix(p, "/") && serve(w, r, clean+"/index.html"):
			case serve(w, r, clean):
			case serve(w, r, clean+"/index.html"):
			default:
				redirect(w, r) // captive-portal behavior for anything unknown
			}
		}
	})

	return mux
}
