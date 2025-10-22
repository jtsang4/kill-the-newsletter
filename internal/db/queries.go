package db

import (
	"context"
	"database/sql"
)

type Feed struct {
	ID        int64
	PublicID  string
	Title     string
	Icon      sql.NullString
	EmailIcon sql.NullString
}

type FeedEntry struct {
	ID        int64
	PublicID  string
	FeedID    int64
	CreatedAt string
	Author    sql.NullString
	Title     string
	Content   string
}

type Enclosure struct {
	ID       int64
	PublicID string
	Type     string
	Length   int64
	Name     string
}

// Feeds
func CreateFeed(ctx context.Context, tx *sql.Tx, publicId, title string) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO feeds(publicId, title) VALUES (?, ?)`, publicId, title)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetFeedByPublicID(ctx context.Context, dbx *sql.DB, pub string) (*Feed, error) {
	row := dbx.QueryRowContext(ctx, `SELECT id, publicId, title, icon, emailIcon FROM feeds WHERE publicId=?`, pub)
	var f Feed
	if err := row.Scan(&f.ID, &f.PublicID, &f.Title, &f.Icon, &f.EmailIcon); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func UpdateFeed(ctx context.Context, tx *sql.Tx, id int64, title string, icon *string) error {
	if icon == nil {
		_, err := tx.ExecContext(ctx, `UPDATE feeds SET title=?, icon=NULL WHERE id=?`, title, id)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE feeds SET title=?, icon=? WHERE id=?`, title, *icon, id)
	return err
}

func DeleteFeed(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM feeds WHERE id=?`, id)
	return err
}

// Entries
func InsertEntry(ctx context.Context, tx *sql.Tx, publicId string, feedID int64, createdAt, author, title, content string) (int64, error) {
	var a *string
	if author != "" {
		a = &author
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO feedEntries(publicId, feed, createdAt, author, title, content) VALUES (?,?,?,?,?,?)`, publicId, feedID, createdAt, a, title, content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func GetFeedEntriesDesc(ctx context.Context, dbx *sql.DB, feedID int64) ([]FeedEntry, error) {
	rows, err := dbx.QueryContext(ctx, `SELECT id, publicId, feed, createdAt, author, title, content FROM feedEntries WHERE feed=? ORDER BY id DESC`, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedEntry
	for rows.Next() {
		var e FeedEntry
		if err := rows.Scan(&e.ID, &e.PublicID, &e.FeedID, &e.CreatedAt, &e.Author, &e.Title, &e.Content); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func GetEntryByPublicID(ctx context.Context, dbx *sql.DB, feedID int64, pub string) (*FeedEntry, error) {
	row := dbx.QueryRowContext(ctx, `SELECT id, publicId, feed, createdAt, author, title, content FROM feedEntries WHERE feed=? AND publicId=?`, feedID, pub)
	var e FeedEntry
	if err := row.Scan(&e.ID, &e.PublicID, &e.FeedID, &e.CreatedAt, &e.Author, &e.Title, &e.Content); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// Enclosures
func InsertEnclosure(ctx context.Context, tx *sql.Tx, publicId, typ string, length int64, name string) (int64, error) {
	res, err := tx.ExecContext(ctx, `INSERT INTO feedEntryEnclosures(publicId, type, length, name) VALUES (?,?,?,?)`, publicId, typ, length, name)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func LinkEnclosure(ctx context.Context, tx *sql.Tx, entryID, enclID int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO feedEntryEnclosureLinks(feedEntry, feedEntryEnclosure) VALUES (?,?)`, entryID, enclID)
	return err
}

func GetEnclosuresForEntry(ctx context.Context, dbx *sql.DB, entryID int64) ([]Enclosure, error) {
	rows, err := dbx.QueryContext(ctx, `SELECT e.publicId, e.type, e.length, e.name, e.id FROM feedEntryEnclosures e JOIN feedEntryEnclosureLinks l ON e.id = l.feedEntryEnclosure WHERE l.feedEntry=?`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Enclosure
	for rows.Next() {
		var enc Enclosure
		if err := rows.Scan(&enc.PublicID, &enc.Type, &enc.Length, &enc.Name, &enc.ID); err != nil {
			return nil, err
		}
		out = append(out, enc)
	}
	return out, rows.Err()
}

// Visualizations & rate limiting
func CountRecentVisualizations(ctx context.Context, dbx *sql.DB, feedID int64, since string) (int64, error) {
	row := dbx.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedVisualizations WHERE feed=? AND ? < createdAt`, feedID, since)
	var c int64
	if err := row.Scan(&c); err != nil {
		return 0, err
	}
	return c, nil
}

func InsertVisualization(ctx context.Context, dbx *sql.DB, feedID int64, createdAt string) error {
	_, err := dbx.ExecContext(ctx, `INSERT INTO feedVisualizations(feed, createdAt) VALUES (?,?)`, feedID, createdAt)
	return err
}

// WebSub
func UpsertWebSubSubscription(ctx context.Context, tx *sql.Tx, feedID int64, createdAt, callback string, secret *string) error {
	// Try update; if no row, insert
	res, err := tx.ExecContext(ctx, `UPDATE feedWebSubSubscriptions SET createdAt=?, secret=? WHERE feed=? AND callback=?`, createdAt, secret, feedID, callback)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, err = tx.ExecContext(ctx, `INSERT INTO feedWebSubSubscriptions(feed, createdAt, callback, secret) VALUES (?,?,?,?)`, feedID, createdAt, callback, secret)
	}
	return err
}

func DeleteWebSubSubscription(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM feedWebSubSubscriptions WHERE id=?`, id)
	return err
}

func GetWebSubSubscriptionsRecent(ctx context.Context, dbx *sql.DB, feedID int64, since string) ([]struct {
	ID       int64
	Callback string
	Secret   *string
}, error) {
	rows, err := dbx.QueryContext(ctx, `SELECT id, callback, secret FROM feedWebSubSubscriptions WHERE feed=? AND ? < createdAt`, feedID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID       int64
		Callback string
		Secret   *string
	}
	for rows.Next() {
		var id int64
		var cb string
		var sec *string
		if err := rows.Scan(&id, &cb, &sec); err != nil {
			return nil, err
		}
		out = append(out, struct {
			ID       int64
			Callback string
			Secret   *string
		}{ID: id, Callback: cb, Secret: sec})
	}
	return out, rows.Err()
}

func GetWebSubSubscriptionByID(ctx context.Context, dbx *sql.DB, id int64) (*struct {
	ID       int64
	Callback string
	Secret   *string
}, error) {
	row := dbx.QueryRowContext(ctx, `SELECT id, callback, secret FROM feedWebSubSubscriptions WHERE id=?`, id)
	var rid int64
	var cb string
	var sec *string
	if err := row.Scan(&rid, &cb, &sec); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &struct {
		ID       int64
		Callback string
		Secret   *string
	}{ID: rid, Callback: cb, Secret: sec}, nil
}

// Background jobs
func EnqueueJob(ctx context.Context, dbx *sql.DB, typ, startAt string, params string) error {
	_, err := dbx.ExecContext(ctx, `INSERT INTO backgroundJobs(type, startAt, parameters, status) VALUES (?,?,?, 'pending')`, typ, startAt, params)
	return err
}

func EnqueueJobTx(ctx context.Context, tx *sql.Tx, typ, startAt string, params string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO backgroundJobs(type, startAt, parameters, status) VALUES (?,?,?, 'pending')`, typ, startAt, params)
	return err
}

func DequeueJob(ctx context.Context, tx *sql.Tx, typ string, now string) (int64, string, error) {
	// Simple FIFO: pick the earliest pending job of given type whose startAt <= now and mark as running
	row := tx.QueryRowContext(ctx, `SELECT id, parameters FROM backgroundJobs WHERE type=? AND status='pending' AND startAt <= ? ORDER BY id ASC LIMIT 1`, typ, now)
	var id int64
	var params string
	if err := row.Scan(&id, &params); err != nil {
		return 0, "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE backgroundJobs SET status='running' WHERE id=?`, id); err != nil {
		return 0, "", err
	}
	return id, params, nil
}

func FinishJob(ctx context.Context, tx *sql.Tx, id int64, ok bool) error {
	status := "done"
	if !ok {
		status = "failed"
	}
	_, err := tx.ExecContext(ctx, `UPDATE backgroundJobs SET status=? WHERE id=?`, status, id)
	return err
}

// Cleanup helpers
func DeleteOldVisualizations(ctx context.Context, dbx *sql.DB, olderThan string) error {
	_, err := dbx.ExecContext(ctx, `DELETE FROM feedVisualizations WHERE createdAt < ?`, olderThan)
	return err
}

func DeleteOldWebSubs(ctx context.Context, dbx *sql.DB, olderThan string) error {
	_, err := dbx.ExecContext(ctx, `DELETE FROM feedWebSubSubscriptions WHERE createdAt < ?`, olderThan)
	return err
}

func GetOrphanEnclosures(ctx context.Context, dbx *sql.DB) ([]struct {
	ID       int64
	PublicID string
}, error) {
	rows, err := dbx.QueryContext(ctx, `SELECT e.id, e.publicId FROM feedEntryEnclosures e LEFT JOIN feedEntryEnclosureLinks l ON e.id = l.feedEntryEnclosure WHERE l.id IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		ID       int64
		PublicID string
	}
	for rows.Next() {
		var id int64
		var pid string
		if err := rows.Scan(&id, &pid); err != nil {
			return nil, err
		}
		out = append(out, struct {
			ID       int64
			PublicID string
		}{ID: id, PublicID: pid})
	}
	return out, rows.Err()
}

func DeleteEnclosureByID(ctx context.Context, dbx *sql.DB, id int64) error {
	_, err := dbx.ExecContext(ctx, `DELETE FROM feedEntryEnclosures WHERE id=?`, id)
	return err
}
