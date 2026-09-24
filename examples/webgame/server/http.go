package server

import (
	"Ginx/gnet"
	"Ginx/limit"
	"Ginx/wsbridge"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// NewHTTPHandler 的访客和诊断接口仅服务于本地 Demo，不是框架管理 API。
func NewHTTPHandler(world *World, tcp *gnet.Server, bridge *wsbridge.Bridge, staticDir string) http.Handler {
	mux := http.NewServeMux()
	guestLimit, _ := limit.NewTokenBucket(2, 10)
	write := func(w http.ResponseWriter, status int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/guest", func(w http.ResponseWriter, r *http.Request) {
		if !guestLimit.Allow() {
			write(w, 429, map[string]string{"error": "too_many_guests"})
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
		if err != nil || !decode(data, &body) {
			write(w, 400, map[string]string{"error": "invalid_request"})
			return
		}
		guest, err := world.Guest(body.Name)
		if err != nil {
			write(w, 400, map[string]string{"error": err.Error()})
			return
		}
		write(w, 200, guest)
	})
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"transport": tcp.GetMetrics(), "tick_rate": TickRate, "snapshot_rate": TickRate / 2, "workers": 4})
	})
	mux.Handle("GET /ws", bridge)
	files := http.FileServer(http.Dir(staticDir))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		}
		if path != "/index.html" && !strings.HasPrefix(path, "/assets/") {
			http.NotFound(w, r)
			return
		}
		info, err := os.Stat(filepath.Join(staticDir, filepath.FromSlash(path)))
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}
