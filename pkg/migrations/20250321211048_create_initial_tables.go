package migrations

import (
	"context"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
)

func init() {
	up := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec(`
			CREATE TABLE jobs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				type TEXT NOT NULL,
				status TEXT NOT NULL,
				data TEXT NOT NULL,
				progress INTEGER NOT NULL,
				process_id TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE libraries (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				name TEXT NOT NULL,
				organize_file_structure BOOLEAN NOT NULL DEFAULT TRUE,
				deleted_at TIMESTAMPTZ
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE library_paths (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_library_paths_library_id ON library_paths (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE series (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				deleted_at TIMESTAMPTZ,
				library_id INTEGER REFERENCES libraries (id) NOT NULL,
				name TEXT NOT NULL,
				name_source TEXT NOT NULL,
				description TEXT,
				cover_image_path TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		// Case-insensitive unique constraint (only for non-deleted records)
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_series_name_library_id ON series (name COLLATE NOCASE, library_id) WHERE deleted_at IS NULL`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_series_library_id ON series (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE books (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER REFERENCES libraries (id) NOT NULL,
				filepath TEXT NOT NULL,
				title TEXT NOT NULL,
				title_source TEXT NOT NULL,
				subtitle TEXT,
				subtitle_source TEXT,
				author_source TEXT NOT NULL,
				cover_image_path TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_books_library_id ON books (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_books_filepath_library_id ON books (filepath, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE book_series (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER REFERENCES books (id) NOT NULL,
				series_id INTEGER REFERENCES series (id) NOT NULL,
				series_number REAL,
				sort_order INTEGER NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_book_series_book_id ON book_series (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_book_series_series_id ON book_series (series_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_book_series_book_series ON book_series (book_id, series_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE files (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id TEXT REFERENCES libraries (id) NOT NULL,
				book_id TEXT REFERENCES books (id) NOT NULL,
				filepath TEXT NOT NULL,
				file_type TEXT NOT NULL,
				filesize_bytes INTEGER NOT NULL,
				cover_mime_type TEXT,
				cover_source TEXT,
				audiobook_duration DOUBLE,
				audiobook_bitrate INTEGER,
				narrator_source TEXT
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_library_id ON files (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_files_book_id ON files (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_files_filepath_library_id ON files (filepath, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE book_identifiers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				book_id TEXT REFERENCES books (id) NOT NULL,
				type TEXT NOT NULL,
				value TEXT NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_book_identifiers_book_id ON book_identifiers (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE persons (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				library_id INTEGER REFERENCES libraries (id) NOT NULL,
				name TEXT NOT NULL,
				sort_name TEXT NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_persons_name_library_id ON persons (name COLLATE NOCASE, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_persons_library_id ON persons (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE authors (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				book_id INTEGER REFERENCES books (id) NOT NULL,
				person_id INTEGER REFERENCES persons (id) NOT NULL,
				sort_order INTEGER NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_authors_book_id ON authors (book_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_authors_person_id ON authors (person_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_authors_book_person ON authors (book_id, person_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE narrators (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				file_id INTEGER REFERENCES files (id) NOT NULL,
				person_id INTEGER REFERENCES persons (id) NOT NULL,
				sort_order INTEGER NOT NULL
			)
`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_narrators_file_id ON narrators (file_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_narrators_person_id ON narrators (person_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_narrators_file_person ON narrators (file_id, person_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Roles and permissions
		_, err = db.Exec(`
			CREATE TABLE roles (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				name TEXT NOT NULL UNIQUE,
				is_system BOOLEAN NOT NULL DEFAULT FALSE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`
			CREATE TABLE permissions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				role_id INTEGER REFERENCES roles (id) ON DELETE CASCADE NOT NULL,
				resource TEXT NOT NULL,
				operation TEXT NOT NULL,
				UNIQUE (role_id, resource, operation)
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_permissions_role_id ON permissions (role_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Users
		_, err = db.Exec(`
			CREATE TABLE users (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
				username TEXT NOT NULL UNIQUE COLLATE NOCASE,
				email TEXT COLLATE NOCASE,
				password_hash TEXT NOT NULL,
				role_id INTEGER REFERENCES roles (id) NOT NULL,
				is_active BOOLEAN NOT NULL DEFAULT TRUE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_users_role_id ON users (role_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_users_email ON users (email) WHERE email IS NOT NULL`)
		if err != nil {
			return errors.WithStack(err)
		}

		// User library access
		_, err = db.Exec(`
			CREATE TABLE user_library_access (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id INTEGER REFERENCES users (id) ON DELETE CASCADE NOT NULL,
				library_id INTEGER REFERENCES libraries (id) ON DELETE CASCADE
			)
		`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_user_library_access_user_id ON user_library_access (user_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE INDEX ix_user_library_access_library_id ON user_library_access (library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`CREATE UNIQUE INDEX ux_user_library_access ON user_library_access (user_id, library_id)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Insert predefined roles
		_, err = db.Exec(`INSERT INTO roles (name, is_system) VALUES ('admin', TRUE)`)
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec(`INSERT INTO roles (name, is_system) VALUES ('viewer', TRUE)`)
		if err != nil {
			return errors.WithStack(err)
		}

		// Get role IDs
		var adminRoleID, viewerRoleID int
		err = db.QueryRow(`SELECT id FROM roles WHERE name = 'admin'`).Scan(&adminRoleID)
		if err != nil {
			return errors.WithStack(err)
		}
		err = db.QueryRow(`SELECT id FROM roles WHERE name = 'viewer'`).Scan(&viewerRoleID)
		if err != nil {
			return errors.WithStack(err)
		}

		// Define all resources and operations
		resources := []string{"libraries", "books", "people", "series", "users", "jobs", "config"}
		operations := []string{"read", "write"}

		// Admin gets all permissions
		for _, resource := range resources {
			for _, operation := range operations {
				_, err = db.Exec(`INSERT INTO permissions (role_id, resource, operation) VALUES (?, ?, ?)`,
					adminRoleID, resource, operation)
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}

		// Viewer gets read-only on libraries, books, series, people
		viewerResources := []string{"libraries", "books", "series", "people"}
		for _, resource := range viewerResources {
			_, err = db.Exec(`INSERT INTO permissions (role_id, resource, operation) VALUES (?, ?, 'read')`,
				viewerRoleID, resource)
			if err != nil {
				return errors.WithStack(err)
			}
		}

		return nil
	}

	down := func(_ context.Context, db *bun.DB) error {
		_, err := db.Exec("DROP TABLE IF EXISTS user_library_access")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS users")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS permissions")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS roles")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS narrators")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS authors")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS persons")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS book_identifiers")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS files")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS book_series")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS books")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS series")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS library_paths")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS libraries")
		if err != nil {
			return errors.WithStack(err)
		}
		_, err = db.Exec("DROP TABLE IF EXISTS jobs")
		return errors.WithStack(err)
	}

	Migrations.MustRegister(up, down)
}
