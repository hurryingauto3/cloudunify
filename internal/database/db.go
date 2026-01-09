package database

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps the sql.DB with application-specific methods
type DB struct {
	*sql.DB
}

// Open creates a new database connection
func Open(dbPath string) (*DB, error) {
	// Ensure parent directory exists
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Run migrations
	if err := RunMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &DB{db}, nil
}

// Provider CRUD operations

// CreateProvider inserts a new provider
func (db *DB) CreateProvider(ctx context.Context, p *Provider) error {
	result, err := db.ExecContext(ctx, `
		INSERT INTO providers (name, type, enabled, quota_bytes, used_bytes, access_token, refresh_token, token_expiry, config)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.Name, p.Type, p.Enabled, p.QuotaBytes, p.UsedBytes, p.AccessToken, p.RefreshToken, p.TokenExpiry, p.Config)
	if err != nil {
		return fmt.Errorf("failed to insert provider: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	p.ID = id
	return nil
}

// GetProvider retrieves a provider by ID
func (db *DB) GetProvider(ctx context.Context, id int64) (*Provider, error) {
	var p Provider
	var tokenExpiry sql.NullTime
	err := db.QueryRowContext(ctx, `
		SELECT id, name, type, enabled, quota_bytes, used_bytes, access_token, refresh_token, token_expiry, config, created_at, updated_at
		FROM providers WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.Type, &p.Enabled, &p.QuotaBytes, &p.UsedBytes, &p.AccessToken, &p.RefreshToken, &tokenExpiry, &p.Config, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if tokenExpiry.Valid {
		p.TokenExpiry = &tokenExpiry.Time
	}
	return &p, nil
}

// ListProviders returns all providers
func (db *DB) ListProviders(ctx context.Context) ([]*Provider, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, type, enabled, quota_bytes, used_bytes, access_token, refresh_token, token_expiry, config, created_at, updated_at
		FROM providers ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}
	defer rows.Close()

	var providers []*Provider
	for rows.Next() {
		var p Provider
		var tokenExpiry sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Enabled, &p.QuotaBytes, &p.UsedBytes, &p.AccessToken, &p.RefreshToken, &tokenExpiry, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		if tokenExpiry.Valid {
			p.TokenExpiry = &tokenExpiry.Time
		}
		providers = append(providers, &p)
	}
	return providers, nil
}

// ListEnabledProviders returns only enabled providers
func (db *DB) ListEnabledProviders(ctx context.Context) ([]*Provider, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, type, enabled, quota_bytes, used_bytes, access_token, refresh_token, token_expiry, config, created_at, updated_at
		FROM providers WHERE enabled = 1 ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled providers: %w", err)
	}
	defer rows.Close()

	var providers []*Provider
	for rows.Next() {
		var p Provider
		var tokenExpiry sql.NullTime
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.Enabled, &p.QuotaBytes, &p.UsedBytes, &p.AccessToken, &p.RefreshToken, &tokenExpiry, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan provider: %w", err)
		}
		if tokenExpiry.Valid {
			p.TokenExpiry = &tokenExpiry.Time
		}
		providers = append(providers, &p)
	}
	return providers, nil
}

// UpdateProvider updates a provider
func (db *DB) UpdateProvider(ctx context.Context, p *Provider) error {
	_, err := db.ExecContext(ctx, `
		UPDATE providers SET name=?, type=?, enabled=?, quota_bytes=?, used_bytes=?, access_token=?, refresh_token=?, token_expiry=?, config=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, p.Name, p.Type, p.Enabled, p.QuotaBytes, p.UsedBytes, p.AccessToken, p.RefreshToken, p.TokenExpiry, p.Config, p.ID)
	if err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}
	return nil
}

// DeleteProvider removes a provider
func (db *DB) DeleteProvider(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM providers WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("failed to delete provider: %w", err)
	}
	return nil
}

// UpdateProviderUsage updates the used_bytes for a provider
func (db *DB) UpdateProviderUsage(ctx context.Context, id int64, usedBytes int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE providers SET used_bytes=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, usedBytes, id)
	if err != nil {
		return fmt.Errorf("failed to update provider usage: %w", err)
	}
	return nil
}

// File CRUD operations

// CreateFile inserts a new file
func (db *DB) CreateFile(ctx context.Context, f *File) error {
	result, err := db.ExecContext(ctx, `
		INSERT INTO files (virtual_path, provider_id, cloud_file_id, cloud_path, size_bytes, checksum, mime_type, status, is_dir)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, f.VirtualPath, f.ProviderID, f.CloudFileID, f.CloudPath, f.SizeBytes, f.Checksum, f.MimeType, f.Status, f.IsDir)
	if err != nil {
		return fmt.Errorf("failed to insert file: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	f.ID = id
	return nil
}

// GetFile retrieves a file by ID
func (db *DB) GetFile(ctx context.Context, id int64) (*File, error) {
	var f File
	err := db.QueryRowContext(ctx, `
		SELECT id, virtual_path, provider_id, cloud_file_id, cloud_path, size_bytes, checksum, mime_type, status, is_dir, created_at, updated_at
		FROM files WHERE id = ?
	`, id).Scan(&f.ID, &f.VirtualPath, &f.ProviderID, &f.CloudFileID, &f.CloudPath, &f.SizeBytes, &f.Checksum, &f.MimeType, &f.Status, &f.IsDir, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	return &f, nil
}

// GetFileByPath retrieves a file by virtual path
func (db *DB) GetFileByPath(ctx context.Context, virtualPath string) (*File, error) {
	var f File
	err := db.QueryRowContext(ctx, `
		SELECT id, virtual_path, provider_id, cloud_file_id, cloud_path, size_bytes, checksum, mime_type, status, is_dir, created_at, updated_at
		FROM files WHERE virtual_path = ?
	`, virtualPath).Scan(&f.ID, &f.VirtualPath, &f.ProviderID, &f.CloudFileID, &f.CloudPath, &f.SizeBytes, &f.Checksum, &f.MimeType, &f.Status, &f.IsDir, &f.CreatedAt, &f.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file by path: %w", err)
	}
	return &f, nil
}

// ListFilesInDirectory returns all files in a directory
func (db *DB) ListFilesInDirectory(ctx context.Context, dirPath string) ([]*File, error) {
	// Normalize path
	if dirPath == "" || dirPath == "/" {
		dirPath = "/"
	}

	// Build pattern for direct children only
	pattern := dirPath
	if !strings.HasSuffix(pattern, "/") {
		pattern += "/"
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, virtual_path, provider_id, cloud_file_id, cloud_path, size_bytes, checksum, mime_type, status, is_dir, created_at, updated_at
		FROM files
		WHERE virtual_path LIKE ? || '%'
		AND virtual_path != ?
		AND instr(substr(virtual_path, length(?) + 1), '/') = 0
		ORDER BY is_dir DESC, virtual_path
	`, pattern, dirPath, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer rows.Close()

	var files []*File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.VirtualPath, &f.ProviderID, &f.CloudFileID, &f.CloudPath, &f.SizeBytes, &f.Checksum, &f.MimeType, &f.Status, &f.IsDir, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan file: %w", err)
		}
		files = append(files, &f)
	}
	return files, nil
}

// UpdateFile updates a file
func (db *DB) UpdateFile(ctx context.Context, f *File) error {
	_, err := db.ExecContext(ctx, `
		UPDATE files SET provider_id=?, cloud_file_id=?, cloud_path=?, size_bytes=?, checksum=?, mime_type=?, status=?, is_dir=?, updated_at=CURRENT_TIMESTAMP
		WHERE id=?
	`, f.ProviderID, f.CloudFileID, f.CloudPath, f.SizeBytes, f.Checksum, f.MimeType, f.Status, f.IsDir, f.ID)
	if err != nil {
		return fmt.Errorf("failed to update file: %w", err)
	}
	return nil
}

// UpdateFileStatus updates only the status of a file
func (db *DB) UpdateFileStatus(ctx context.Context, id int64, status FileStatus) error {
	_, err := db.ExecContext(ctx, `
		UPDATE files SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, status, id)
	if err != nil {
		return fmt.Errorf("failed to update file status: %w", err)
	}
	return nil
}

// DeleteFile removes a file
func (db *DB) DeleteFile(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM files WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// DeleteFileByPath removes a file by its virtual path
func (db *DB) DeleteFileByPath(ctx context.Context, virtualPath string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM files WHERE virtual_path=?", virtualPath)
	if err != nil {
		return fmt.Errorf("failed to delete file by path: %w", err)
	}
	return nil
}

// Sync Queue operations

// EnqueueSync adds an item to the sync queue
func (db *DB) EnqueueSync(ctx context.Context, item *SyncQueueItem) error {
	result, err := db.ExecContext(ctx, `
		INSERT INTO sync_queue (operation, virtual_path, local_path, provider_id, priority, status)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.Operation, item.VirtualPath, item.LocalPath, item.ProviderID, item.Priority, item.Status)
	if err != nil {
		return fmt.Errorf("failed to enqueue sync: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	item.ID = id
	return nil
}

// DequeueSync gets the next pending item from the queue
func (db *DB) DequeueSync(ctx context.Context) (*SyncQueueItem, error) {
	return db.DequeueSyncByOperation(ctx, "")
}

// DequeueSyncByOperation gets the next pending item for a specific operation
func (db *DB) DequeueSyncByOperation(ctx context.Context, operation string) (*SyncQueueItem, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var item SyncQueueItem
	var query string
	var args []interface{}

	if operation != "" {
		query = `
			SELECT id, operation, virtual_path, local_path, provider_id, priority, status, progress_percent, COALESCE(error_message, ''), retry_count, created_at, updated_at
			FROM sync_queue
			WHERE status = 'pending' AND operation = ?
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
		`
		args = []interface{}{operation}
	} else {
		query = `
			SELECT id, operation, virtual_path, local_path, provider_id, priority, status, progress_percent, COALESCE(error_message, ''), retry_count, created_at, updated_at
			FROM sync_queue
			WHERE status = 'pending'
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
		`
	}

	err = tx.QueryRowContext(ctx, query, args...).Scan(&item.ID, &item.Operation, &item.VirtualPath, &item.LocalPath, &item.ProviderID, &item.Priority, &item.Status, &item.ProgressPercent, &item.ErrorMessage, &item.RetryCount, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue sync: %w", err)
	}

	// Mark as processing
	_, err = tx.ExecContext(ctx, `
		UPDATE sync_queue SET status='processing', updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, item.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to mark as processing: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	item.Status = SyncStatusProcessing
	return &item, nil
}

// UpdateSyncProgress updates the progress of a sync queue item
func (db *DB) UpdateSyncProgress(ctx context.Context, id int64, progress int) error {
	_, err := db.ExecContext(ctx, `
		UPDATE sync_queue SET progress_percent=?, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, progress, id)
	if err != nil {
		return fmt.Errorf("failed to update sync progress: %w", err)
	}
	return nil
}

// CompleteSyncItem marks a sync queue item as completed
func (db *DB) CompleteSyncItem(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE sync_queue SET status='completed', progress_percent=100, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, id)
	if err != nil {
		return fmt.Errorf("failed to complete sync item: %w", err)
	}
	return nil
}

// FailSyncItem marks a sync queue item as failed
func (db *DB) FailSyncItem(ctx context.Context, id int64, errorMsg string) error {
	_, err := db.ExecContext(ctx, `
		UPDATE sync_queue SET status='failed', error_message=?, retry_count=retry_count+1, updated_at=CURRENT_TIMESTAMP WHERE id=?
	`, errorMsg, id)
	if err != nil {
		return fmt.Errorf("failed to fail sync item: %w", err)
	}
	return nil
}

// RetryFailedSync resets a failed item to pending for retry
func (db *DB) RetryFailedSync(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE sync_queue SET status='pending', error_message='', updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='failed'
	`, id)
	if err != nil {
		return fmt.Errorf("failed to retry sync: %w", err)
	}
	return nil
}

// ListSyncQueue returns items in the sync queue
func (db *DB) ListSyncQueue(ctx context.Context, status SyncStatus) ([]*SyncQueueItem, error) {
	query := `
		SELECT id, operation, virtual_path, local_path, provider_id, priority, status, progress_percent, COALESCE(error_message, ''), retry_count, created_at, updated_at
		FROM sync_queue
	`
	var args []interface{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY priority DESC, created_at ASC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sync queue: %w", err)
	}
	defer rows.Close()

	var items []*SyncQueueItem
	for rows.Next() {
		var item SyncQueueItem
		if err := rows.Scan(&item.ID, &item.Operation, &item.VirtualPath, &item.LocalPath, &item.ProviderID, &item.Priority, &item.Status, &item.ProgressPercent, &item.ErrorMessage, &item.RetryCount, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan sync item: %w", err)
		}
		items = append(items, &item)
	}
	return items, nil
}

// DeleteSyncItem removes an item from the sync queue
func (db *DB) DeleteSyncItem(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sync_queue WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("failed to delete sync item: %w", err)
	}
	return nil
}

// ClearCompletedSync removes all completed sync items
func (db *DB) ClearCompletedSync(ctx context.Context) error {
	_, err := db.ExecContext(ctx, "DELETE FROM sync_queue WHERE status='completed'")
	if err != nil {
		return fmt.Errorf("failed to clear completed sync: %w", err)
	}
	return nil
}

// Cache operations

// CreateCacheEntry adds a cache entry
func (db *DB) CreateCacheEntry(ctx context.Context, c *CacheEntry) error {
	result, err := db.ExecContext(ctx, `
		INSERT INTO cache (file_id, local_path, size_bytes)
		VALUES (?, ?, ?)
	`, c.FileID, c.LocalPath, c.SizeBytes)
	if err != nil {
		return fmt.Errorf("failed to create cache entry: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	c.ID = id
	return nil
}

// GetCacheEntry retrieves a cache entry by file ID
func (db *DB) GetCacheEntry(ctx context.Context, fileID int64) (*CacheEntry, error) {
	var c CacheEntry
	err := db.QueryRowContext(ctx, `
		SELECT id, file_id, local_path, size_bytes, last_accessed
		FROM cache WHERE file_id = ?
	`, fileID).Scan(&c.ID, &c.FileID, &c.LocalPath, &c.SizeBytes, &c.LastAccessed)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get cache entry: %w", err)
	}
	return &c, nil
}

// TouchCacheEntry updates the last accessed time
func (db *DB) TouchCacheEntry(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, `
		UPDATE cache SET last_accessed=CURRENT_TIMESTAMP WHERE id=?
	`, id)
	if err != nil {
		return fmt.Errorf("failed to touch cache entry: %w", err)
	}
	return nil
}

// DeleteCacheEntry removes a cache entry
func (db *DB) DeleteCacheEntry(ctx context.Context, id int64) error {
	_, err := db.ExecContext(ctx, "DELETE FROM cache WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("failed to delete cache entry: %w", err)
	}
	return nil
}

// GetCacheSizeBytes returns the total size of cached files
func (db *DB) GetCacheSizeBytes(ctx context.Context) (int64, error) {
	var size sql.NullInt64
	err := db.QueryRowContext(ctx, "SELECT SUM(size_bytes) FROM cache").Scan(&size)
	if err != nil {
		return 0, fmt.Errorf("failed to get cache size: %w", err)
	}
	if !size.Valid {
		return 0, nil
	}
	return size.Int64, nil
}

// GetLRUCacheEntries returns cache entries ordered by last accessed (oldest first)
func (db *DB) GetLRUCacheEntries(ctx context.Context, limit int) ([]*CacheEntry, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, file_id, local_path, size_bytes, last_accessed
		FROM cache
		ORDER BY last_accessed ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get LRU cache entries: %w", err)
	}
	defer rows.Close()

	var entries []*CacheEntry
	for rows.Next() {
		var c CacheEntry
		if err := rows.Scan(&c.ID, &c.FileID, &c.LocalPath, &c.SizeBytes, &c.LastAccessed); err != nil {
			return nil, fmt.Errorf("failed to scan cache entry: %w", err)
		}
		entries = append(entries, &c)
	}
	return entries, nil
}

// Storage summary

// GetStorageSummary returns aggregated storage statistics
func (db *DB) GetStorageSummary(ctx context.Context) (*StorageSummary, error) {
	providers, err := db.ListProviders(ctx)
	if err != nil {
		return nil, err
	}

	var summary StorageSummary
	for _, p := range providers {
		summary.TotalBytes += p.QuotaBytes
		summary.UsedBytes += p.UsedBytes

		// Count files for this provider
		var count int
		err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM files WHERE provider_id=?", p.ID).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("failed to count files: %w", err)
		}

		summary.Providers = append(summary.Providers, ProviderSummary{
			Provider:  p,
			FileCount: count,
		})
	}
	summary.FreeBytes = summary.TotalBytes - summary.UsedBytes

	return &summary, nil
}

// Helper to get filename from path
func getFileName(path string) string {
	return filepath.Base(path)
}

// StartCleanupRoutine starts a background routine to clean up old completed sync items
func (db *DB) StartCleanupRoutine(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Delete completed items older than 24 hours
				db.ExecContext(ctx, `
					DELETE FROM sync_queue
					WHERE status='completed'
					AND updated_at < datetime('now', '-24 hours')
				`)
			}
		}
	}()
}
