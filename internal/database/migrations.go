package database

import (
	"database/sql"
	"fmt"
)

// Migration represents a database migration
type Migration struct {
	Version int
	Up      string
	Down    string
}

// migrations contains all database migrations in order
var migrations = []Migration{
	{
		Version: 1,
		Up: `
			-- Providers table stores cloud storage provider configurations
			CREATE TABLE IF NOT EXISTS providers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				type TEXT NOT NULL,
				enabled INTEGER DEFAULT 1,
				quota_bytes INTEGER,
				used_bytes INTEGER DEFAULT 0,
				access_token TEXT,
				refresh_token TEXT,
				token_expiry DATETIME,
				config TEXT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
			);

			-- Files table stores virtual filesystem file metadata
			CREATE TABLE IF NOT EXISTS files (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				virtual_path TEXT NOT NULL UNIQUE,
				provider_id INTEGER NOT NULL,
				cloud_file_id TEXT NOT NULL,
				cloud_path TEXT,
				size_bytes INTEGER NOT NULL,
				checksum TEXT,
				mime_type TEXT,
				status TEXT NOT NULL DEFAULT 'pending',
				is_dir INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (provider_id) REFERENCES providers(id)
			);

			-- Indexes for files table
			CREATE INDEX IF NOT EXISTS idx_files_virtual_path ON files(virtual_path);
			CREATE INDEX IF NOT EXISTS idx_files_provider ON files(provider_id);
			CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);

			-- Sync queue table for tracking pending operations
			CREATE TABLE IF NOT EXISTS sync_queue (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				operation TEXT NOT NULL,
				virtual_path TEXT NOT NULL,
				local_path TEXT,
				provider_id INTEGER,
				priority INTEGER DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'pending',
				progress_percent INTEGER DEFAULT 0,
				error_message TEXT,
				retry_count INTEGER DEFAULT 0,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (provider_id) REFERENCES providers(id)
			);

			-- Index for sync queue to efficiently fetch pending items
			CREATE INDEX IF NOT EXISTS idx_sync_queue_status ON sync_queue(status, priority DESC);

			-- Cache table for tracking locally cached files
			CREATE TABLE IF NOT EXISTS cache (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				file_id INTEGER NOT NULL,
				local_path TEXT NOT NULL,
				size_bytes INTEGER NOT NULL,
				last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (file_id) REFERENCES files(id)
			);

			-- Index for cache eviction (LRU)
			CREATE INDEX IF NOT EXISTS idx_cache_last_accessed ON cache(last_accessed);

			-- Schema version tracking
			CREATE TABLE IF NOT EXISTS schema_version (
				version INTEGER PRIMARY KEY
			);
			INSERT INTO schema_version (version) VALUES (1);
		`,
		Down: `
			DROP TABLE IF EXISTS cache;
			DROP TABLE IF EXISTS sync_queue;
			DROP TABLE IF EXISTS files;
			DROP TABLE IF EXISTS providers;
			DROP TABLE IF EXISTS schema_version;
		`,
	},
	{
		Version: 2,
		Up: `
			ALTER TABLE files ADD COLUMN pinned INTEGER DEFAULT 0;
		`,
		Down: `
			-- No downgrade supported for this column drop in sqlite
		`,
	},
}

// getCurrentVersion returns the current schema version
func getCurrentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT version FROM schema_version ORDER BY version DESC LIMIT 1").Scan(&version)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		// Table might not exist yet
		return 0, nil
	}
	return version, nil
}

// RunMigrations applies all pending migrations
func RunMigrations(db *sql.DB) error {
	currentVersion, err := getCurrentVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", migration.Version, err)
		}

		if _, err := tx.Exec(migration.Up); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		// Update schema version (for migrations after v1)
		if migration.Version > 1 {
			if _, err := tx.Exec("UPDATE schema_version SET version = ?", migration.Version); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to update schema version: %w", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// RollbackMigration rolls back the last migration (for development use)
func RollbackMigration(db *sql.DB) error {
	currentVersion, err := getCurrentVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	if currentVersion == 0 {
		return fmt.Errorf("no migrations to rollback")
	}

	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		if migration.Version != currentVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for rollback: %w", err)
		}

		if _, err := tx.Exec(migration.Down); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit rollback: %w", err)
		}

		return nil
	}

	return fmt.Errorf("migration %d not found", currentVersion)
}
