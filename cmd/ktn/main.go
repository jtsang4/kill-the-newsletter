package main

import (
	"context"
	"flag"
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
	var cfgPath string
	var typ string
	var httpAddr string
	flag.StringVar(&cfgPath, "config", "configs/development.json", "Path to JSON config file")
	flag.StringVar(&typ, "type", "all", "Which component to run: server|email|background|all")
	flag.StringVar(&httpAddr, "http-addr", ":8080", "HTTP listen address")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
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
	if typ == "server" || typ == "all" {
		hs := httpserver.New(cfg, dbx)
		httpSrv = &http.Server{Addr: httpAddr, Handler: hs}
		go func() {
			log.Printf("HTTP listening on %s", httpAddr)
			if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("http: %v", err)
			}
		}()
	}

	var smtpSrv interface{ Close() error }
	if typ == "email" || typ == "all" {
		ss, err := smtpserver.Start(cfg, dbx)
		if err != nil {
			log.Fatalf("smtp: %v", err)
		}
		smtpSrv = ss
	}

	if typ == "background" || typ == "all" {
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
