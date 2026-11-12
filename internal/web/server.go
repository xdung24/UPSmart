package web

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"upsmart/internal/state"
)

//go:embed static/*
var assets embed.FS

func NewServer(address string, store *state.Store) *http.Server {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/app.js", "/style.css":
			files.ServeHTTP(w, r)
		case "/api/v1/status":
			writeJSON(w, http.StatusOK, store.Snapshot(time.Now()))
		case "/api/v1/hardware":
			writeJSON(w, http.StatusOK, store.HardwareSnapshot(time.Now()))
		case "/healthz":
			writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
		case "/readyz":
			snapshot := store.Snapshot(time.Now())
			if snapshot.Telemetry == nil || snapshot.Telemetry.Stale || snapshot.Connection.State != "connected" {
				writeError(w, http.StatusServiceUnavailable, "not_ready", "No fresh verified UPS telemetry")
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		case "/api/v1/events":
			query, err := url.ParseQuery(r.URL.RawQuery)
			if err != nil {
				writeError(w, 400, "invalid_parameter", "Invalid query encoding")
				return
			}
			for key, values := range query {
				if (key != "limit" && key != "before") || len(values) != 1 {
					writeError(w, 400, "invalid_parameter", "Unknown or repeated query parameter")
					return
				}
			}
			limit := 50
			if query.Has("limit") {
				limit, err = strconv.Atoi(query.Get("limit"))
				if err != nil || limit < 1 || limit > 200 {
					writeError(w, 400, "invalid_parameter", "limit must be between 1 and 200")
					return
				}
			}
			page, err := store.Page(limit, query.Get("before"))
			if err != nil {
				code := http.StatusBadRequest
				if errors.Is(err, state.ErrExpiredCursor) {
					code = http.StatusGone
				}
				writeError(w, code, "invalid_cursor", err.Error())
				return
			}
			writeJSON(w, 200, page)
		default:
			writeError(w, 404, "not_found", "Route not found")
		}
	})
	slots := make(chan struct{}, 32)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w = headWriter{w}
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != address {
			writeError(w, 403, "invalid_host", "Use the configured loopback address")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+address {
			writeError(w, 403, "invalid_origin", "Cross-origin access is not allowed")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeError(w, 405, "method_not_allowed", "This server is read-only")
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			writeError(w, 503, "busy", "Too many concurrent requests")
			return
		}
		mux.ServeHTTP(w, r)
	})
	return &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second, WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
}

type headWriter struct{ http.ResponseWriter }

func (w headWriter) Write(p []byte) (int, error) { return len(p), nil }

func writeJSON(w http.ResponseWriter, status int, value any) {
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(value); err != nil {
		log.Printf("encode API response: %v", err)
		writeError(w, 500, "encoding_failed", "Unable to encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write(buffer.Bytes()); err != nil {
		log.Printf("write HTTP response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error state.APIError `json:"error"`
	}{state.APIError{Code: code, Message: message}})
}
