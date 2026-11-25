package main

import (
	"context"
	"database/sql"
	"strings"

	"github.com/pkg/errors"
	"github.com/shishobooks/shisho/pkg/sqliteconn"
)

// readingRecord is the harness user's reading record on the server, for the
// flows to check that what the app changed offline arrived as it was left.
type readingRecord struct {
	// Outcomes are the latest Read-through's outcome of each Book asked
	// about, by title, or empty.
	Outcomes map[string]string `json:"outcomes"`
	// ToRead is To Read's titles, in order.
	ToRead []string `json:"to_read"`
	// Lists are the user's Lists by name, each with its Books' titles in
	// order.
	Lists map[string][]string `json:"lists"`
	// Positions is whether each Book asked about has a Reading Position.
	Positions map[string]bool `json:"positions"`
	// ListeningSeconds is the user's listening time across every Session.
	ListeningSeconds int `json:"listening_seconds"`
}

// readRecord reads the harness user's reading record, with the Books
// titled titles, from the server's database, read-only.
func readRecord(ctx context.Context, path string, titles []string) (*readingRecord, error) {
	connector, err := sqliteconn.NewConnector("file:"+path+"?mode=ro", "PRAGMA busy_timeout = 5000")
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	defer db.Close()

	record := &readingRecord{Outcomes: map[string]string{}, ToRead: []string{}, Lists: map[string][]string{}, Positions: map[string]bool{}}
	for _, title := range titles {
		var outcome string
		err := db.QueryRowContext(ctx, `
			SELECT rt.outcome FROM read_throughs rt JOIN books b ON b.id = rt.book_id JOIN users u ON u.id = rt.user_id
			WHERE b.title = ? AND u.username = ? ORDER BY rt.id DESC LIMIT 1`, title, harnessUsername).Scan(&outcome)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, errors.WithStack(err)
		}
		record.Outcomes[title] = outcome
		var positions int
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM reading_positions rp JOIN files f ON f.id = rp.file_id JOIN books b ON b.id = f.book_id
			JOIN users u ON u.id = rp.user_id WHERE b.title = ? AND u.username = ?`, title, harnessUsername).Scan(&positions)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		record.Positions[title] = positions > 0
	}
	queued, err := queryStrings(ctx, db, `
		SELECT b.title FROM user_books ub JOIN books b ON b.id = ub.book_id JOIN users u ON u.id = ub.user_id
		WHERE u.username = ? AND ub.to_read_position IS NOT NULL ORDER BY ub.to_read_position, b.id`, harnessUsername)
	if err != nil {
		return nil, err
	}
	record.ToRead = queued
	names, err := queryStrings(ctx, db, `SELECT l.name FROM lists l JOIN users u ON u.id = l.user_id WHERE u.username = ? ORDER BY l.name`,
		harnessUsername)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		books, err := queryStrings(ctx, db, `
			SELECT b.title FROM list_books lb JOIN lists l ON l.id = lb.list_id JOIN books b ON b.id = lb.book_id
			JOIN users u ON u.id = l.user_id
			WHERE u.username = ? AND l.name = ? ORDER BY lb.sort_order IS NULL, lb.sort_order, lb.added_at DESC`, harnessUsername, name)
		if err != nil {
			return nil, err
		}
		record.Lists[name] = books
	}
	err = db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(rs.duration), 0) FROM reading_sessions rs JOIN users u ON u.id = rs.user_id
		WHERE u.username = ? AND rs.kind = 'listening'`, harnessUsername).Scan(&record.ListeningSeconds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return record, nil
}

func queryStrings(ctx context.Context, db *sql.DB, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, errors.WithStack(err)
		}
		out = append(out, value)
	}
	return out, errors.WithStack(rows.Err())
}

// resetRecord takes the Books titled titles back to unread for the harness
// user, off To Read and unrated, and deletes their Lists named lists, so a
// flow that checks what it changes starts from the same place whatever an
// earlier flow left. The change log's triggers record it, so the feed sends
// it.
func resetRecord(ctx context.Context, path string, titles, lists []string) error {
	db, err := openServerDB(path)
	if err != nil {
		return err
	}
	defer db.Close()
	const user = `(SELECT id FROM users WHERE username = ?)`
	for _, title := range titles {
		for _, statement := range []string{
			`DELETE FROM read_throughs WHERE user_id = ` + user + ` AND book_id IN (SELECT id FROM books WHERE title = ?)`,
			`DELETE FROM user_books WHERE user_id = ` + user + ` AND book_id IN (SELECT id FROM books WHERE title = ?)`,
		} {
			if _, err := db.ExecContext(ctx, statement, harnessUsername, title); err != nil {
				return errors.WithStack(err)
			}
		}
	}
	for _, name := range lists {
		if _, err := db.ExecContext(ctx, `DELETE FROM lists WHERE user_id = `+user+` AND name = ?`, harnessUsername, name); err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// splitParam splits a comma-separated query parameter, dropping blanks.
func splitParam(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
