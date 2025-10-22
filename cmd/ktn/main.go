package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jtsang4/kill-the-newsletter/internal/config"
	"github.com/jtsang4/kill-the-newsletter/internal/db"
	"github.com/jtsang4/kill-the-newsletter/internal/httpserver"
	"github.com/jtsang4/kill-the-newsletter/internal/smtpserver"
	"github.com/jtsang4/kill-the-newsletter/internal/worker"
)

func main() {
	cfg, err := config.LoadEnv()
	if err != nil {
		log.Fatalf("load config from env: %v", err)
	}
	if err := os.MkdirAll(cfg.DataDirectory, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	dbx, err := db.Open(cfg.DataDirectory)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer dbx.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var httpSrv *http.Server
	if cfg.RunType == "server" || cfg.RunType == "all" {
		hs := httpserver.New(cfg, dbx)
		httpSrv = &http.Server{Addr: cfg.HTTPAddr, Handler: hs}
		go func() {
			log.Printf("HTTP listening on %s", cfg.HTTPAddr)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("http: %v", err)
			}
		}()
	}

	var smtpSrv interface{ Close() error }
	if cfg.RunType == "email" || cfg.RunType == "all" {
		ss, err := smtpserver.Start(cfg, dbx)
		if err != nil {
			log.Fatalf("smtp: %v", err)
		}
		smtpSrv = ss
	}

	if cfg.RunType == "background" || cfg.RunType == "all" {
		worker.Start(ctx, cfg, dbx)
	}

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	if httpSrv != nil {
		_ = httpSrv.Shutdown(shutdownCtx)
	}
	if smtpSrv != nil {
		_ = smtpSrv.Close()
	}
}
