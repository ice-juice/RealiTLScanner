package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/xtls/RealiTLScanner/internal/desktop"
)

// HTTP + SSE fallback when Wails is not used. Native window: `wails build` / `wails dev` from repo root.

func main() {
	app := desktop.NewApp()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app.Startup(ctx)

	hub := newSSEHub()
	app.SetEmitter(hub)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/defaults", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, app.GetDefaults())
	})
	mux.HandleFunc("/api/validate", func(w http.ResponseWriter, r *http.Request) {
		var req desktop.ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "无效的 JSON", http.StatusBadRequest)
			return
		}
		writeJSON(w, app.ValidateConfig(req))
	})
	mux.HandleFunc("/api/start", func(w http.ResponseWriter, r *http.Request) {
		var req desktop.ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "无效的 JSON", http.StatusBadRequest)
			return
		}
		if err := app.StartScan(req); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]string{"status": "started"})
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, _ *http.Request) {
		_ = app.StopScan()
		writeJSON(w, map[string]string{"status": "stopping"})
	})
	mux.HandleFunc("/api/geodb", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, app.GeoDBStatus())
	})
	mux.HandleFunc("/api/geodb/download", func(w http.ResponseWriter, _ *http.Request) {
		if err := app.DownloadGeoDB(); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, app.GeoDBStatus())
	})
	mux.HandleFunc("/api/open-folder", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		if err := app.OpenOutputFolder(path); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/events", hub.ServeHTTP)
	mux.Handle("/", frontendHandler())

	addr := ":34115"
	if v := os.Getenv("REALITL_DESKTOP_ADDR"); v != "" {
		addr = v
	}
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	fmt.Printf("RealiTLScanner desktop backend listening on http://127.0.0.1%s\n", addr)
	fmt.Println("Native window: from repo root run `wails dev` or `wails build`.")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func frontendHandler() http.Handler {
	for _, dir := range []string{
		"frontend/dist",
		filepath.Join(exeDir(), "frontend", "dist"),
	} {
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err == nil {
			return http.FileServer(http.Dir(dir))
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			http.ServeFile(w, r, fallbackPath())
			return
		}
		http.FileServer(http.Dir("frontend")).ServeHTTP(w, r)
	})
}

func fallbackPath() string {
	for _, p := range []string{
		"frontend/fallback.html",
		filepath.Join(exeDir(), "frontend", "fallback.html"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "frontend/fallback.html"
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

type sseHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newSSEHub() *sseHub {
	return &sseHub{clients: map[chan string]struct{}{}}
}

func (h *sseHub) Emit(name string, payload any) {
	b, err := json.Marshal(map[string]any{"event": name, "data": payload})
	if err != nil {
		return
	}
	msg := "data: " + string(b) + "\n\n"
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

func (h *sseHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan string, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			_, _ = fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}
