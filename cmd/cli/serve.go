package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"cleanpoint/internal/api"
	"cleanpoint/internal/config"
	"cleanpoint/internal/storage"
)

// serverMain mirrors cmd/server so `cleanpoint serve` works from one binary.
func serverMain() {
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
	httpSrv := &http.Server{Addr: cfg.Addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
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

func runtimeVersion() string { return runtime.Version() }
