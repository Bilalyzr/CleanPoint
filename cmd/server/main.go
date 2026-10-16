// Command server runs the CleanPoint local API server.
//
// CleanPoint is a local, defensively designed ransomware-recovery analysis
// prototype. It binds to loopback by default; exposing it beyond loopback is
// explicitly unsupported.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cleanpoint/internal/api"
	"cleanpoint/internal/config"
	"cleanpoint/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer db.Close()

	srv := api.NewServer(cfg, storage.NewStore(db))
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("CleanPoint local simulation API listening on http://%s (sandbox=%s)", cfg.Addr, cfg.SandboxRoot)
	log.Printf("NOTE: local simulation only. No production data-protection guarantees. See docs/limitations.md")

	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}
