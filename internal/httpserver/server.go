package httpserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jtsang4/kill-the-newsletter/internal/atom"
	"github.com/jtsang4/kill-the-newsletter/internal/config"
	"github.com/jtsang4/kill-the-newsletter/internal/db"
	"github.com/jtsang4/kill-the-newsletter/internal/util"
)

type Server struct {
	cfg       config.Config
	db        *db.DB
	mux       *http.ServeMux
	templates *template.Template
}

type Option func(*Server)

func New(cfg config.Config, dbx *db.DB, opts ...Option) *Server {
	s := &Server{cfg: cfg, db: dbx, mux: http.NewServeMux()}
	t := template.New("")
	t = template.Must(t.ParseFS(templatesFS, "*.html"))
	s.templates = t
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("/", s.handleHome)
	s.mux.HandleFunc("/feeds", s.handleFeeds)
	s.mux.HandleFunc("/feeds/", s.handleFeedsSub)
	// static passthrough for icons
	s.mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("static", "favicon.ico"))
	})
	s.mux.HandleFunc("/apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join("static", "apple-touch-icon.png"))
	})
	// files mapping
	s.mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(filepath.Join(s.cfg.DataDirectory, "files")))))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) ListenAndServe(addr string) error {
	server := &http.Server{Addr: addr, Handler: s}
	log.Printf("HTTP server listening on %s", addr)
	return server.ListenAndServe()
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.notFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.render(w, "index.html", map[string]any{"Hostname": s.cfg.Hostname})
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			s.serverError(w, r, err)
			return
		}
		title := strings.TrimSpace(r.Form.Get("title"))
		if title == "" || len(title) > 200 {
			s.validationError(w, r, "invalid title")
			return
		}
		ctx := r.Context()
		err := s.db.Tx(ctx, func(tx *db.Tx) error {
			pid, _ := util.RandID(20)
			_, err := db.CreateFeed(ctx, tx, pid, title)
			if err != nil {
				return err
			}
			if strings.Contains(r.Header.Get("Accept"), "application/json") {
				resp := map[string]string{
					"feedId": pid,
					"email":  fmt.Sprintf("%s@%s", pid, s.cfg.Hostname),
					"feed":   fmt.Sprintf("https://%s/feeds/%s.xml", s.cfg.Hostname, pid),
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
				return nil
			}
			http.Redirect(w, r, "/feeds/"+pid, http.StatusFound)
			return nil
		})
		if err != nil {
			s.serverError(w, r, err)
			return
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

var feedIDRe = regexp.MustCompile(`^/feeds/([A-Za-z0-9]+)$`)
var feedXMLRe = regexp.MustCompile(`^/feeds/([A-Za-z0-9]+)\.xml$`)
var feedEntryHTMLRe = regexp.MustCompile(`^/feeds/([A-Za-z0-9]+)/entries/([A-Za-z0-9]+)\.html$`)
var feedWebSubRe = regexp.MustCompile(`^/feeds/([A-Za-z0-9]+)/websub$`)

func (s *Server) handleFeeds(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/feeds" {
		s.notFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// delegate to home POST for creation (same validation)
	s.handleHome(w, r)
}

func (s *Server) handleFeedsSub(w http.ResponseWriter, r *http.Request) {
	if m := feedIDRe.FindStringSubmatch(r.URL.Path); m != nil {
		s.handleFeedPage(w, r, m[1])
		return
	}
	if m := feedXMLRe.FindStringSubmatch(r.URL.Path); m != nil {
		s.handleFeedXML(w, r, m[1])
		return
	}
	if m := feedEntryHTMLRe.FindStringSubmatch(r.URL.Path); m != nil {
		s.handleFeedEntryHTML(w, r, m[1], m[2])
		return
	}
	if m := feedWebSubRe.FindStringSubmatch(r.URL.Path); m != nil {
		s.handleWebSub(w, r, m[1])
		return
	}
	s.notFound(w, r)
}

func (s *Server) handleFeedPage(w http.ResponseWriter, r *http.Request, pub string) {
	ctx := r.Context()
	f, err := db.GetFeedByPublicID(ctx, s.db.SQL, pub)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if f == nil {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.render(w, "feed.html", map[string]any{"Feed": f, "Hostname": s.cfg.Hostname})
	case http.MethodPatch:
		if err := r.ParseForm(); err != nil {
			s.serverError(w, r, err)
			return
		}
		title := strings.TrimSpace(r.Form.Get("title"))
		icon := strings.TrimSpace(r.Form.Get("icon"))
		if title == "" || len(title) > 200 {
			s.validationError(w, r, "invalid title")
			return
		}
		var iconPtr *string
		if icon != "" {
			if len(icon) > 200 {
				s.validationError(w, r, "invalid icon")
				return
			}
			iconPtr = &icon
		}
		err := s.db.Tx(ctx, func(tx *db.Tx) error { return db.UpdateFeed(ctx, tx, f.ID, title, iconPtr) })
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	case http.MethodDelete:
		err := s.db.Tx(ctx, func(tx *db.Tx) error { return db.DeleteFeed(ctx, tx, f.ID) })
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleFeedXML(w http.ResponseWriter, r *http.Request, pub string) {
	ctx := r.Context()
	f, err := db.GetFeedByPublicID(ctx, s.db.SQL, pub)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if f == nil {
		return
	}
	w.Header().Set("X-Robots-Tag", "none")
	// rate limit: 10 visualizations/hour
	since := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)
	count, err := db.CountRecentVisualizations(ctx, s.db.SQL, f.ID, since)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if count > 10 {
		w.WriteHeader(http.StatusTooManyRequests)
		s.render(w, "rate_limit.html", nil)
		return
	}
	_ = db.InsertVisualization(ctx, s.db.SQL, f.ID, time.Now().UTC().Format(time.RFC3339Nano))
	entries, err := db.GetFeedEntriesDesc(ctx, s.db.SQL, f.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	var items []atom.Entry
	for _, e := range entries {
		encls, err := db.GetEnclosuresForEntry(ctx, s.db.SQL, e.ID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		var arr []atom.Enclosure
		for _, x := range encls {
			arr = append(arr, atom.Enclosure{PublicID: x.PublicID, Type: x.Type, Length: x.Length, Name: x.Name})
		}
		var author *string
		if e.Author.Valid {
			author = &e.Author.String
		}
		items = append(items, atom.Entry{ID: e.ID, PublicID: e.PublicID, CreatedAt: e.CreatedAt, Author: author, Title: e.Title, Content: e.Content, Enclosures: arr})
	}
	var icon *string
	if f.Icon.Valid {
		icon = &f.Icon.String
	}
	var emailIcon *string
	if f.EmailIcon.Valid {
		emailIcon = &f.EmailIcon.String
	}
	xmlStr, err := atom.BuildFeedXML(s.cfg.Hostname, atom.Feed{PublicID: f.PublicID, Title: f.Title, Icon: icon, EmailIcon: emailIcon}, items)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	_, _ = w.Write([]byte(xmlStr))
}

func (s *Server) handleFeedEntryHTML(w http.ResponseWriter, r *http.Request, pub, entryPub string) {
	ctx := r.Context()
	f, err := db.GetFeedByPublicID(ctx, s.db.SQL, pub)
	if err != nil || f == nil {
		return
	}
	e, err := db.GetEntryByPublicID(ctx, s.db.SQL, f.ID, entryPub)
	if err != nil || e == nil {
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src *; style-src 'self' 'unsafe-inline'; frame-src 'none'; object-src 'none'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("Cross-Origin-Embedder-Policy", "unsafe-none")
	_, _ = w.Write([]byte(e.Content))
}

func (s *Server) handleWebSub(w http.ResponseWriter, r *http.Request, pub string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	f, err := db.GetFeedByPublicID(ctx, s.db.SQL, pub)
	if err != nil || f == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.serverError(w, r, err)
		return
	}
	mode := r.Form.Get("hub.mode")
	if mode == "" {
		mode = r.Form.Get("hub.url") /* fallback parity */
	}
	topic := r.Form.Get("hub.topic")
	if topic == "" {
		topic = r.Form.Get("hub.url")
	}
	callback := r.Form.Get("hub.callback")
	secret := r.Form.Get("hub.secret")
	if !(mode == "subscribe" || mode == "unsubscribe") {
		s.validationError(w, r, "invalid mode")
		return
	}
	if topic != fmt.Sprintf("https://%s/feeds/%s.xml", s.cfg.Hostname, f.PublicID) {
		s.validationError(w, r, "invalid topic")
		return
	}
	if callback == "" {
		s.validationError(w, r, "invalid callback")
		return
	}
	if u := strings.ToLower(callback); !(strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
		s.validationError(w, r, "invalid callback")
		return
	}
	if host := hostOf(callback); host == s.cfg.Hostname || host == "localhost" || host == "127.0.0.1" {
		s.validationError(w, r, "invalid callback host")
		return
	}
	if secret != "" && len(secret) == 0 {
		s.validationError(w, r, "invalid secret")
		return
	}
	// naive daily limit: ensure <= 10 different callbacks in last 24h (done at verify time in original, here we enforce on enqueue)
	since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339Nano)
	subs, err := db.GetWebSubSubscriptionsRecent(ctx, s.db.SQL, f.ID, since)
	if err == nil && mode == "subscribe" && uniqueCallbacks(subs) > 10 && !containsCallback(subs, callback) {
		s.validationError(w, r, "rate limited")
		return
	}
	var secretPtr *string
	if strings.TrimSpace(secret) != "" {
		secretPtr = &secret
	}
	params, _ := json.Marshal(map[string]any{"feedId": f.ID, "hub.mode": mode, "hub.topic": topic, "hub.callback": callback, "hub.secret": secretPtr})
	_ = db.EnqueueJob(ctx, s.db.SQL, "feedWebSubSubscriptions.verify", time.Now().UTC().Format(time.RFC3339Nano), string(params))
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		log.Println("render error:", err)
	}
}

func (s *Server) validationError(w http.ResponseWriter, _ *http.Request, msg string) {
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(msg))
}

func (s *Server) serverError(w http.ResponseWriter, _ *http.Request, err error) {
	log.Println("server error:", err)
	s.render(w, "error.html", nil)
}

func (s *Server) notFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	s.render(w, "not_found.html", nil)
}

func hostOf(u string) string {
	i := strings.Index(u, "://")
	if i == -1 {
		return ""
	}
	rest := u[i+3:]
	j := strings.IndexAny(rest, "/?")
	if j == -1 {
		return rest
	}
	return rest[:j]
}

func uniqueCallbacks(v []struct {
	ID       int64
	Callback string
	Secret   *string
}) int {
	m := map[string]struct{}{}
	for _, x := range v {
		m[x.Callback] = struct{}{}
	}
	return len(m)
}

func containsCallback(v []struct {
	ID       int64
	Callback string
	Secret   *string
}, cb string) bool {
	for _, x := range v {
		if x.Callback == cb {
			return true
		}
	}
	return false
}

// Utility to compute X-Hub-Signature
func ComputeXHubSignature(secret, body string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
