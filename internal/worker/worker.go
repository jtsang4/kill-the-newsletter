package worker

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jtsang4/kill-the-newsletter/internal/atom"
	"github.com/jtsang4/kill-the-newsletter/internal/config"
	"github.com/jtsang4/kill-the-newsletter/internal/db"
)

type VerifyJob struct {
	FeedID      int64   `json:"feedId"`
	HubMode     string  `json:"hub.mode"`
	HubTopic    string  `json:"hub.topic"`
	HubCallback string  `json:"hub.callback"`
	HubSecret   *string `json:"hub.secret"`
}

type DispatchJob struct {
	FeedID                   int64 `json:"feedId"`
	FeedEntryID              int64 `json:"feedEntryId"`
	FeedWebSubSubscriptionID int64 `json:"feedWebSubSubscriptionId"`
}

func Start(ctx context.Context, cfg config.Config, dbx *db.DB) {
	// Verify workers (parallel 8)
	for i := 0; i < 8; i++ {
		go verifyLoop(ctx, cfg, dbx)
	}
	// Dispatch workers (parallel 4)
	for i := 0; i < 4; i++ {
		go dispatchLoop(ctx, cfg, dbx)
	}
	// Cleanup ticker
	go cleanupLoop(ctx, dbx, cfg)
}

func verifyLoop(ctx context.Context, cfg config.Config, dbx *db.DB) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var id int64
		var params string
		// Dequeue and mark as running inside a short transaction
		_ = dbx.Tx(ctx, func(tx *db.Tx) error {
			jid, p, err := db.DequeueJob(ctx, tx, "feedWebSubSubscriptions.verify", time.Now().UTC().Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
			id, params = jid, p
			return nil
		})
		if id == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Parse outside transaction
		var job VerifyJob
		if err := json.Unmarshal([]byte(params), &job); err != nil {
			_ = dbx.Tx(ctx, func(tx *db.Tx) error { return db.FinishJob(ctx, tx, id, false) })
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Process outside transaction
		ok := processVerify(ctx, cfg, dbx, job)
		// Finish in a short transaction
		_ = dbx.Tx(ctx, func(tx *db.Tx) error { return db.FinishJob(ctx, tx, id, ok) })
		time.Sleep(50 * time.Millisecond)
	}
}

func processVerify(ctx context.Context, cfg config.Config, dbx *db.DB, job VerifyJob) bool {
	f, err := db.GetFeedByID(ctx, dbx.SQL, job.FeedID)
	if err != nil || f == nil {
		return false
	}
	challenge := fmt.Sprintf("%x", sha256.Sum256([]byte(time.Now().String())))
	u, err := url.Parse(job.HubCallback)
	if err != nil {
		return false
	}
	q := u.Query()
	q.Set("hub.mode", job.HubMode)
	q.Set("hub.topic", job.HubTopic)
	q.Set("hub.challenge", challenge)
	if job.HubMode == "subscribe" {
		q.Set("hub.lease_seconds", strconv.Itoa(24*60*60))
	}
	u.RawQuery = q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != challenge {
		return false
	}
	return dbx.Tx(ctx, func(tx *db.Tx) error {
		if job.HubMode == "subscribe" {
			return db.UpsertWebSubSubscription(ctx, tx, f.ID, time.Now().UTC().Format(time.RFC3339Nano), job.HubCallback, job.HubSecret)
		}
		// unsubscribe
		sub, err := db.GetWebSubSubscriptionsRecentTx(ctx, tx, f.ID, "1970-01-01T00:00:00Z")
		if err != nil {
			return err
		}
		for _, s := range sub {
			if s.Callback == job.HubCallback {
				return db.DeleteWebSubSubscription(ctx, tx, s.ID)
			}
		}
		return nil
	}) == nil
}

func dispatchLoop(ctx context.Context, cfg config.Config, dbx *db.DB) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var id int64
		var params string
		// Dequeue and mark as running inside a short transaction
		_ = dbx.Tx(ctx, func(tx *db.Tx) error {
			jid, p, err := db.DequeueJob(ctx, tx, "feedWebSubSubscriptions.dispatch", time.Now().UTC().Format(time.RFC3339Nano))
			if err != nil {
				return err
			}
			id, params = jid, p
			return nil
		})
		if id == 0 {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Parse outside transaction
		var job DispatchJob
		if err := json.Unmarshal([]byte(params), &job); err != nil {
			_ = dbx.Tx(ctx, func(tx *db.Tx) error { return db.FinishJob(ctx, tx, id, false) })
			time.Sleep(50 * time.Millisecond)
			continue
		}
		// Process outside transaction (performs DB reads and network I/O)
		ok := processDispatch(ctx, cfg, dbx, job)
		// Finish in a short transaction
		_ = dbx.Tx(ctx, func(tx *db.Tx) error { return db.FinishJob(ctx, tx, id, ok) })
		time.Sleep(50 * time.Millisecond)
	}
}

func processDispatch(ctx context.Context, cfg config.Config, dbx *db.DB, job DispatchJob) bool {
	feed, err := db.GetFeedByID(ctx, dbx.SQL, job.FeedID)
	if err != nil || feed == nil {
		return false
	}
	entry, err := db.GetEntryByID(ctx, dbx.SQL, job.FeedEntryID)
	if err != nil || entry == nil {
		return false
	}
	sub, err := db.GetWebSubSubscriptionByID(ctx, dbx.SQL, job.FeedWebSubSubscriptionID)
	if err != nil || sub == nil {
		return false
	}
	// Build one-entry Atom body
	var icon *string
	if feed.Icon.Valid {
		icon = &feed.Icon.String
	}
	var emailIcon *string
	if feed.EmailIcon.Valid {
		emailIcon = &feed.EmailIcon.String
	}
	var author *string
	if entry.Author.Valid {
		author = &entry.Author.String
	}
	encls, _ := db.GetEnclosuresForEntry(ctx, dbx.SQL, entry.ID)
	var arr []atom.Enclosure
	for _, e := range encls {
		arr = append(arr, atom.Enclosure{PublicID: e.PublicID, Type: e.Type, Length: e.Length, Name: e.Name})
	}
	body, err := atom.BuildFeedXML(cfg.Hostname, atom.Feed{PublicID: feed.PublicID, Title: feed.Title, Icon: icon, EmailIcon: emailIcon}, []atom.Entry{{ID: entry.ID, PublicID: entry.PublicID, CreatedAt: entry.CreatedAt, Author: author, Title: entry.Title, Content: entry.Content, Enclosures: arr}})
	if err != nil {
		return false
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sub.Callback, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/atom+xml; charset=utf-8")
	req.Header.Set("Link", fmt.Sprintf("<https://%s/feeds/%s.xml>; rel=\"self\", <https://%s/feeds/%s/websub>; rel=\"hub\"", cfg.Hostname, feed.PublicID, cfg.Hostname, feed.PublicID))
	if sub.Secret != nil {
		h := hmac.New(sha256.New, []byte(*sub.Secret))
		h.Write([]byte(body))
		req.Header.Set("X-Hub-Signature", "sha256="+hex.EncodeToString(h.Sum(nil)))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusGone {
		_ = dbx.Tx(ctx, func(tx *db.Tx) error { return db.DeleteWebSubSubscription(ctx, tx, sub.ID) })
		return true
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		log.Printf("websub dispatch client error: %s", resp.Status)
		return true
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func cleanupLoop(ctx context.Context, dbx *db.DB, cfg config.Config) {
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		olderViz := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)
		_ = db.DeleteOldVisualizations(ctx, dbx.SQL, olderViz)
		olderSubs := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
		_ = db.DeleteOldWebSubs(ctx, dbx.SQL, olderSubs)
		// delete orphan enclosure files and records
		orphans, _ := db.GetOrphanEnclosures(ctx, dbx.SQL)
		for _, o := range orphans {
			_ = os.RemoveAll(filepath.Join(cfg.DataDirectory, "files", o.PublicID))
			_ = db.DeleteEnclosureByID(ctx, dbx.SQL, o.ID)
		}
	}
}

// helpers
func urlQueryEscape(s string) string { return (&url.URL{Path: s}).EscapedPath() }
