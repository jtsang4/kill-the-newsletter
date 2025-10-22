package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jtsang4/kill-the-newsletter/internal/config"
	"github.com/jtsang4/kill-the-newsletter/internal/db"
	"github.com/jtsang4/kill-the-newsletter/internal/httpserver"
	"github.com/jtsang4/kill-the-newsletter/internal/smtpserver"
	"github.com/jtsang4/kill-the-newsletter/internal/worker"

	"net/smtp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tmpDir, err := os.MkdirTemp("", "ktn-e2e-*")
	if err != nil {
		log.Fatalf("tmpdir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	httpPort := freePort()
	smtpPort := freePort()

	cfg := config.Config{
		Hostname:      "127.0.0.1",
		TLS:           config.TLS{},
		DataDirectory: filepath.Join(tmpDir, "data"),
		Environment:   string(config.EnvDevelopment),
		SMTPPort:      smtpPort,
	}
	if err := os.MkdirAll(cfg.DataDirectory, 0o755); err != nil {
		log.Fatalf("mkdir data: %v", err)
	}

	dbx, err := db.Open(cfg.DataDirectory)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer dbx.Close()

	hs := httpserver.New(cfg, dbx)
	httpAddr := fmt.Sprintf("127.0.0.1:%d", httpPort)
	httpSrv := &http.Server{Addr: httpAddr, Handler: hs}
	go func() {
		log.Printf("HTTP server listening on %s", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	smtpSrv, err := smtpserver.Start(cfg, dbx)
	if err != nil {
		log.Fatalf("smtp start: %v", err)
	}
	defer smtpSrv.Close()

	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()
	worker.Start(workerCtx, cfg, dbx)

	waitForHTTP(httpAddr)
	waitForSMTP(cfg.SMTPPort)

	feedID := createFeed(httpAddr)
	log.Printf("created feed %s", feedID)

	sendEmail(cfg.SMTPPort, feedID, cfg.Hostname)
	log.Printf("sent email to %s@%s", feedID, cfg.Hostname)

	entryTitle := "Test Newsletter"
	if err := waitForFeed(httpAddr, feedID, entryTitle, 10*time.Second); err != nil {
		log.Fatalf("feed wait: %v", err)
	}
	log.Printf("feed entry with title %q detected", entryTitle)

	if err := verifyEntryHTML(httpAddr, dbx, feedID); err != nil {
		log.Fatalf("entry html: %v", err)
	}
	log.Println("entry HTML verified")

	log.Println("E2E test passed")
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitForHTTP(addr string) {
	url := fmt.Sprintf("http://%s/", addr)
	for i := 0; i < 50; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Fatalf("http server not reachable at %s", addr)
}

func waitForSMTP(port int) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 50; i++ {
		conn, err := net.DialTimeout("tcp", addr, 150*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	log.Fatalf("smtp server not reachable at %s", addr)
}

func createFeed(httpAddr string) string {
	endpoint := fmt.Sprintf("http://%s/", httpAddr)
	form := url.Values{}
	form.Set("title", "Example Feed")
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		log.Fatalf("create feed request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("create feed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Fatalf("create feed status=%d body=%s", resp.StatusCode, string(body))
	}
	var payload struct {
		FeedID string `json:"feedId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Fatalf("decode create feed: %v", err)
	}
	return payload.FeedID
}

func sendEmail(port int, feedID, hostname string) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	recipient := fmt.Sprintf("%s@%s", feedID, hostname)
	log.Printf("connecting to SMTP %s", addr)
	d := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		log.Fatalf("smtp dial: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	c, err := smtp.NewClient(conn, hostname)
	if err != nil {
		log.Fatalf("smtp new client: %v", err)
	}
	defer c.Close()
	if err := c.Hello("localhost"); err != nil {
		log.Fatalf("smtp hello: %v", err)
	}
	if ok, _ := c.Extension("STARTTLS"); ok {
		log.Fatalf("smtp STARTTLS unexpectedly advertised")
	}
	if err := c.Mail("sender@example.com"); err != nil {
		log.Fatalf("smtp MAIL: %v", err)
	}
	if err := c.Rcpt(recipient); err != nil {
		log.Fatalf("smtp RCPT: %v", err)
	}
	wc, err := c.Data()
	if err != nil {
		log.Fatalf("smtp DATA: %v", err)
	}
	msg := strings.Join([]string{
		"From: \"Sender\" <sender@example.com>",
		"To: \"Feed\" <" + recipient + ">",
		"Subject: Test Newsletter",
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=utf-8",
		"",
		"<p>Hello <strong>World</strong></p>",
	}, "\r\n") + "\r\n"
	if _, err := io.WriteString(wc, msg); err != nil {
		log.Fatalf("smtp write body: %v", err)
	}
	if err := wc.Close(); err != nil {
		log.Fatalf("smtp close data: %v", err)
	}
	if err := c.Quit(); err != nil {
		log.Fatalf("smtp quit: %v", err)
	}
}

func waitForFeed(httpAddr, feedID, title string, timeout time.Duration) error {
	url := fmt.Sprintf("http://%s/feeds/%s.xml", httpAddr, feedID)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr == nil && strings.Contains(string(body), title) {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("feed %s did not contain %q within %s", feedID, title, timeout)
}

func verifyEntryHTML(httpAddr string, dbx *db.DB, feedID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	feed, err := db.GetFeedByPublicID(ctx, dbx.SQL, feedID)
	if err != nil {
		return fmt.Errorf("load feed: %w", err)
	}
	entries, err := db.GetFeedEntriesDesc(ctx, dbx.SQL, feed.ID)
	if err != nil {
		return fmt.Errorf("entries: %w", err)
	}
	if len(entries) == 0 {
		return fmt.Errorf("no entries found for feed %s", feedID)
	}
	entry := entries[0]
	url := fmt.Sprintf("http://%s/feeds/%s/entries/%s.html", httpAddr, feedID, entry.PublicID)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("get entry html: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("entry status=%d body=%s", resp.StatusCode, string(body))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read entry html: %w", err)
	}
	if !strings.Contains(string(body), "Hello") {
		return fmt.Errorf("entry html missing expected content: %s", string(body))
	}
	return nil
}
