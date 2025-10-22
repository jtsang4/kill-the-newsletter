package smtpserver

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/jhillyerd/enmime"

	"github.com/jtsang4/kill-the-newsletter/internal/config"
	"github.com/jtsang4/kill-the-newsletter/internal/db"
	"github.com/jtsang4/kill-the-newsletter/internal/util"
)

type Backend struct {
	cfg config.Config
	db  *db.DB
}

type session struct {
	b     *Backend
	ctx   context.Context
	from  string
	rcpts []string
}

func NewBackend(cfg config.Config, dbx *db.DB) *Backend { return &Backend{cfg: cfg, db: dbx} }

func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &session{b: b, ctx: context.Background()}, nil
}

func (s *session) AuthPlain(username, password string) error {
	return errors.New("auth unsupported")
}

func (s *session) Mail(from string, opts *smtp.MailOptions) error {
	// Enforce basic validations
	if from == "" || (!util.EmailRe.MatchString(from) && s.b.cfg.Environment != string(config.EnvDevelopment)) {
		return errors.New("invalid mailFrom")
	}
	if strings.HasSuffix(strings.ToLower(from), "@blogtrottr.com") || strings.HasSuffix(strings.ToLower(from), "@feedrabbit.com") {
		return errors.New("sender domain not allowed")
	}
	s.from = from
	return nil
}

func (s *session) Rcpt(to string, _ *smtp.RcptOptions) error {
	// only accept recipients for our hostname
	addr := strings.ToLower(strings.TrimSpace(to))
	parts := strings.Split(addr, "@")
	if len(parts) != 2 {
		return errors.New("invalid rcpt")
	}
	if parts[1] != strings.ToLower(s.b.cfg.Hostname) {
		return errors.New("invalid domain")
	}
	if s.b.cfg.Environment != string(config.EnvDevelopment) && !util.EmailRe.MatchString(addr) {
		return errors.New("invalid rcpt format")
	}
	s.rcpts = append(s.rcpts, addr)
	return nil
}

func (s *session) Data(r io.Reader) error {
	// Enforce max size ~ 512KB
	lr := &io.LimitedReader{R: r, N: 1 << 19}
	env, err := enmime.ReadEnvelope(lr)
	if err != nil {
		return err
	}
	if lr.N <= 0 {
		return errors.New("email too big")
	}
	// Map recipients to feeds
	feeds := make([]db.Feed, 0)
	for _, rcpt := range s.rcpts {
		pub := strings.Split(rcpt, "@")[0]
		f, err := db.GetFeedByPublicID(s.ctx, s.b.db.SQL, pub)
		if err != nil {
			return err
		}
		if f != nil {
			feeds = append(feeds, *f)
		}
	}
	if len(feeds) == 0 {
		return errors.New("no valid recipients")
	}
	// Prepare attachments (both attachments and inlines)
	attachments := append([]*enmime.Part{}, env.Attachments...)
	attachments = append(attachments, env.Inlines...)
	for _, f := range feeds {
		if err := s.b.db.Tx(s.ctx, func(tx *db.Tx) error {
			// update emailIcon using sender domain favicon
			domain := strings.Split(strings.ToLower(s.from), "@")[1]
			if err := db.UpdateFeedEmailIcon(s.ctx, tx, f.ID, fmt.Sprintf("https://%s/favicon.ico", domain)); err != nil {
				return err
			}
			// store enclosures
			enclosureIDs := make([]int64, 0, len(attachments))
			for _, a := range attachments {
				// resolve filename
				name := a.FileName
				if name == "" {
					if cd := a.Header.Get("Content-Disposition"); cd != "" {
						_, params, _ := mime.ParseMediaType(cd)
						name = params["filename"]
					}
				}
				name = util.SanitizeFilename(name)
				pid, _ := util.RandID(20)
				length := int64(len(a.Content))
				id, err := db.InsertEnclosure(s.ctx, tx, pid, a.ContentType, length, name)
				if err != nil {
					return err
				}
				// write file
				dir := filepath.Join(s.b.cfg.DataDirectory, "files", pid)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, name), a.Content, 0o644); err != nil {
					return err
				}
				enclosureIDs = append(enclosureIDs, id)
			}
			// create entry
			pid, _ := util.RandID(20)
			createdAt := time.Now().UTC().Format(time.RFC3339Nano)
			author := s.from
			title := env.GetHeader("Subject")
			if strings.TrimSpace(title) == "" {
				title = "Untitled"
			}
			htmlBody := env.HTML
			if strings.TrimSpace(htmlBody) == "" {
				if env.Text != "" {
					htmlBody = "<pre>" + html.EscapeString(env.Text) + "</pre>"
				} else {
					htmlBody = "No content."
				}
			}
			entryID, err := db.InsertEntry(s.ctx, tx, pid, f.ID, createdAt, author, title, htmlBody)
			if err != nil {
				return err
			}
			for _, eid := range enclosureIDs {
				if err := db.LinkEnclosure(s.ctx, tx, entryID, eid); err != nil {
					return err
				}
			}
			// trim feed to ~512KB
			entriesAsc, err := db.GetAllEntriesAscTx(s.ctx, tx, f.ID)
			if err != nil {
				return err
			}
			var size int
			for i := len(entriesAsc) - 1; i >= 0; i-- { // from newest backwards
				size += len(entriesAsc[i].Title) + len(entriesAsc[i].Content)
				if size > 1<<19 { // stop here; entries [0..i] should be deleted
					for j := 0; j <= i; j++ {
						if err := db.DeleteEnclosureLinksByEntry(s.ctx, tx, entriesAsc[j].ID); err != nil {
							return err
						}
						if err := db.DeleteEntryByID(s.ctx, tx, entriesAsc[j].ID); err != nil {
							return err
						}
					}
					break
				}
			}
			// enqueue websub dispatch for last 24h subscriptions
			subs, err := db.GetWebSubSubscriptionsRecentTx(s.ctx, tx, f.ID, time.Now().Add(-24*time.Hour).UTC().Format(time.RFC3339Nano))
			if err == nil {
				for _, sub := range subs {
					params := fmt.Sprintf(`{"feedId":%d,"feedEntryId":%d,"feedWebSubSubscriptionId":%d}`, f.ID, entryID, sub.ID)
					_ = db.EnqueueJobTx(s.ctx, tx, "feedWebSubSubscriptions.dispatch", time.Now().UTC().Format(time.RFC3339Nano), params)
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	log.Printf("EMAIL SUCCESS from=%s feeds=%d", s.from, len(feeds))
	return nil
}

func (s *session) Reset()        { s.from = ""; s.rcpts = nil }
func (s *session) Logout() error { return nil }

// Start launches an SMTP server (port :25) with STARTTLS (if cert available) and AUTH disabled.
func Start(cfg config.Config, dbx *db.DB) (*smtp.Server, error) {
	be := NewBackend(cfg, dbx)
	s := smtp.NewServer(be)
	s.Addr = fmt.Sprintf(":%d", cfg.SMTPPort)
	s.Domain = cfg.Hostname
	s.AllowInsecureAuth = false
	s.AuthDisabled = true
	s.ReadTimeout = 10 * time.Minute
	s.WriteTimeout = 10 * time.Minute
	// TLS
	if cfg.TLS.Key != "" && cfg.TLS.Certificate != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLS.Certificate, cfg.TLS.Key)
		if err == nil {
			s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}} // STARTTLS
		}
	}
	go func() {
		log.Printf("SMTP server listening on %s", s.Addr)
		if err := s.ListenAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Println("smtp error:", err)
		}
	}()
	return s, nil
}

// no helper stubs; direct os operations are used for file I/O
