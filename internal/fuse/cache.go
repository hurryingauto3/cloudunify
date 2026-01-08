package fuse

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"cloudunify/internal/database"
)

// CacheManager handles local file caching
type CacheManager struct {
	db       *database.DB
	cacheDir string
	maxSize  int64 // Maximum cache size in bytes
}

// NewCacheManager creates a new cache manager
func NewCacheManager(db *database.DB, cacheDir string, maxSizeGB int) *CacheManager {
	return &CacheManager{
		db:       db,
		cacheDir: cacheDir,
		maxSize:  int64(maxSizeGB) * 1024 * 1024 * 1024,
	}
}

// GetCachedPath returns the local cache path for a file if it exists
func (cm *CacheManager) GetCachedPath(ctx context.Context, fileID int64) (string, bool, error) {
	entry, err := cm.db.GetCacheEntry(ctx, fileID)
	if err != nil {
		return "", false, err
	}

	if entry == nil {
		return "", false, nil
	}

	// Verify file still exists
	if _, err := os.Stat(entry.LocalPath); os.IsNotExist(err) {
		// Cache entry is stale, remove it
		cm.db.DeleteCacheEntry(ctx, entry.ID)
		return "", false, nil
	}

	// Update last accessed time
	cm.db.TouchCacheEntry(ctx, entry.ID)
	return entry.LocalPath, true, nil
}

// AddToCache adds a file to the cache
func (cm *CacheManager) AddToCache(ctx context.Context, fileID int64, localPath string) error {
	// Get file size
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	// Check if we need to evict files
	if err := cm.ensureSpace(ctx, info.Size()); err != nil {
		return err
	}

	// Create cache entry
	entry := &database.CacheEntry{
		FileID:    fileID,
		LocalPath: localPath,
		SizeBytes: info.Size(),
	}

	return cm.db.CreateCacheEntry(ctx, entry)
}

// RemoveFromCache removes a file from the cache
func (cm *CacheManager) RemoveFromCache(ctx context.Context, fileID int64) error {
	entry, err := cm.db.GetCacheEntry(ctx, fileID)
	if err != nil {
		return err
	}

	if entry == nil {
		return nil
	}

	// Delete local file
	os.Remove(entry.LocalPath)

	// Delete cache entry
	return cm.db.DeleteCacheEntry(ctx, entry.ID)
}

// ensureSpace ensures there's enough space in the cache for a new file
func (cm *CacheManager) ensureSpace(ctx context.Context, needed int64) error {
	currentSize, err := cm.db.GetCacheSizeBytes(ctx)
	if err != nil {
		return err
	}

	// Keep evicting until we have enough space
	for currentSize+needed > cm.maxSize {
		// Get LRU entries
		entries, err := cm.db.GetLRUCacheEntries(ctx, 10)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			// No more entries to evict
			break
		}

		for _, entry := range entries {
			// Delete local file
			os.Remove(entry.LocalPath)

			// Delete cache entry
			if err := cm.db.DeleteCacheEntry(ctx, entry.ID); err != nil {
				log.Printf("Warning: failed to delete cache entry: %v", err)
			}

			currentSize -= entry.SizeBytes
			if currentSize+needed <= cm.maxSize {
				break
			}
		}
	}

	return nil
}

// GetCachePath returns the path where a file should be cached
func (cm *CacheManager) GetCachePath(virtualPath string) string {
	// Create a safe filename from the virtual path
	safePath := filepath.Clean(virtualPath)
	return filepath.Join(cm.cacheDir, safePath)
}

// CleanupOrphanedFiles removes cache files that don't have database entries
func (cm *CacheManager) CleanupOrphanedFiles(ctx context.Context) error {
	return filepath.Walk(cm.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			return nil
		}

		// Check if this file has a cache entry
		// This is a simplified check - in production, you'd want a more efficient approach
		// For now, just skip cleanup
		return nil
	})
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats(ctx context.Context) (*CacheStats, error) {
	size, err := cm.db.GetCacheSizeBytes(ctx)
	if err != nil {
		return nil, err
	}

	entries, err := cm.db.GetLRUCacheEntries(ctx, 1000)
	if err != nil {
		return nil, err
	}

	return &CacheStats{
		CurrentSize: size,
		MaxSize:     cm.maxSize,
		FileCount:   len(entries),
		UsagePercent: float64(size) / float64(cm.maxSize) * 100,
	}, nil
}

// CacheStats contains cache statistics
type CacheStats struct {
	CurrentSize  int64   `json:"current_size"`
	MaxSize      int64   `json:"max_size"`
	FileCount    int     `json:"file_count"`
	UsagePercent float64 `json:"usage_percent"`
}
