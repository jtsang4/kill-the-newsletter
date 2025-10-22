package db

import (
	"context"
	"database/sql"
)

func GetFeedByID(ctx context.Context, dbx *sql.DB, id int64) (*Feed, error) {
	row := dbx.QueryRowContext(ctx, `SELECT id, publicId, title, icon, emailIcon FROM feeds WHERE id=?`, id)
	var f Feed
	if err := row.Scan(&f.ID, &f.PublicID, &f.Title, &f.Icon, &f.EmailIcon); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func GetEntryByID(ctx context.Context, dbx *sql.DB, id int64) (*FeedEntry, error) {
	row := dbx.QueryRowContext(ctx, `SELECT id, publicId, feed, createdAt, author, title, content FROM feedEntries WHERE id=?`, id)
	var e FeedEntry
	if err := row.Scan(&e.ID, &e.PublicID, &e.FeedID, &e.CreatedAt, &e.Author, &e.Title, &e.Content); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func UpdateFeedEmailIcon(ctx context.Context, tx *sql.Tx, feedID int64, emailIcon string) error {
	_, err := tx.ExecContext(ctx, `UPDATE feeds SET emailIcon=? WHERE id=?`, emailIcon, feedID)
	return err
}

func GetWebSubSubscriptionsRecentTx(ctx context.Context, tx *sql.Tx, feedID int64, since string) ([]struct {
	ID       int64
	Callback string
	Secret   *string
}, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, callback, secret FROM feedWebSubSubscriptions WHERE feed=? AND ? < createdAt`, feedID, since)
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

func GetAllEntriesAscTx(ctx context.Context, tx *sql.Tx, feedID int64) ([]FeedEntry, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, publicId, feed, createdAt, author, title, content FROM feedEntries WHERE feed=? ORDER BY id ASC`, feedID)
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

func DeleteEntryByID(ctx context.Context, tx *sql.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM feedEntries WHERE id=?`, id)
	return err
}

func DeleteEnclosureLinksByEntry(ctx context.Context, tx *sql.Tx, entryID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM feedEntryEnclosureLinks WHERE feedEntry=?`, entryID)
	return err
}
