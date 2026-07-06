package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"preview-gateway/internal/children"
	"preview-gateway/internal/config"
	"preview-gateway/internal/health"
	"preview-gateway/internal/paths"
	"preview-gateway/internal/preflight"
	"preview-gateway/internal/proxy"
)

func main() {
	cfg := config.Load()
	p := paths.Resolve(
		cfg.RepoRoot,
		cfg.MobileDir,
		cfg.MobileDataDir,
		cfg.PluginWebDir,
		cfg.SimverseFrontendDir,
		cfg.AirBin,
		cfg.NodeBin,
	)

	if err := preflight.Run(p); err != nil {
		log.Fatalf("preflight failed: %v", err)
	}

	mgr := children.New(p, cfg.SpawnGo, cfg.SpawnVite, cfg.SpawnPluginVite, cfg.SpawnOpenlist, cfg.SpawnSimverseVite)

	if mgr.HasChildren() {
		if err := mgr.StartAll(); err != nil {
			log.Printf("children start failed: %v", err)
			mgr.StopAll()
			os.Exit(1)
		}
	}

	gw := proxy.New()
	hc := health.New()

	mux := http.NewServeMux()
	mux.HandleFunc("/__gateway/health", hc.ServeHTTP)
	mux.HandleFunc("/__gateway/children", func(w http.ResponseWriter, r *http.Request) {
		status := mgr.Status()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\"ok\":true,\"children\":{")
		first := true
		for name, alive := range status {
			if !first {
				w.Write([]byte(","))
			}
			first = false
			fmt.Fprintf(w, "\"%s\":%t", name, alive)
		}
		w.Write([]byte("}}"))
	})
	mux.HandleFunc("/__gateway", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("preview-gateway (Go v1)\n\n" +
			"Endpoints:\n" +
			"  GET /__gateway/health   - health check\n" +
			"  GET /__gateway/children - child process status\n"))
	})
	mux.Handle("/", gw)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		sig := <-ch
		log.Printf("Received signal %s, shutting down...", sig)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}

		mgr.StopAll()
		os.Exit(0)
	}()

	log.Printf("=========================================")
	log.Printf("  preview-gateway (Go) starting")
	log.Printf("  Listening on %s", addr)
	log.Printf("=========================================")

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
