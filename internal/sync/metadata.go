package sync

import (
	"context"
	"fmt"
	"log"
	"path"
	"strings"
	"time"

	"cloudunify/internal/database"
	"cloudunify/internal/providers"
)

// StartMetadataSync starts the periodic metadata sync
func (e *Engine) StartMetadataSync(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial sync
	e.SyncAllProviders(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.SyncAllProviders(ctx)
		}
	}
}

// SyncAllProviders syncs metadata for all registered providers
func (e *Engine) SyncAllProviders(ctx context.Context) {
	e.providersMu.RLock()
	providerIDs := make([]int64, 0, len(e.providers))
	for id := range e.providers {
		providerIDs = append(providerIDs, id)
	}
	e.providersMu.RUnlock()

	for _, id := range providerIDs {
		if err := e.SyncMetadata(ctx, id); err != nil {
			log.Printf("Metadata sync failed for provider %d: %v", id, err)
		}
	}
}

// SyncMetadata performs a full metadata sync for a provider
func (e *Engine) SyncMetadata(ctx context.Context, providerID int64) error {
	e.providersMu.RLock()
	provider, ok := e.providers[providerID]
	e.providersMu.RUnlock()

	if !ok {
		// Provider might check db if not loaded?
		// For now assume loaded.
		return nil
	}

	log.Printf("Starting metadata sync for provider %d", providerID)
	return e.syncFolder(ctx, provider, providerID, "root", "/")
}

func (e *Engine) syncFolder(ctx context.Context, provider providers.CloudProvider, providerID int64, remoteID, virtualPath string) error {
	// List cloud files
	cloudFiles, err := provider.ListFiles(ctx, remoteID)
	if err != nil {
		return err
	}

	// List local DB files
	dbFiles, err := e.db.ListFilesInDirectory(ctx, virtualPath)
	if err != nil {
		return err
	}

	dbFileMap := make(map[string]*database.File)
	for _, f := range dbFiles {
		// Get filename from virtual path
		_, name := path.Split(f.VirtualPath)
		if name == "" {
			name = f.VirtualPath // Should not happen for files
		}
		dbFileMap[name] = f
	}

	// Process cloud files
	seenNames := make(map[string]int)

	for _, cf := range cloudFiles {
		// Handle duplicate filenames (Google Drive allows them, we don't)
		name := cf.Name
		count := seenNames[name]
		if count > 0 {
			ext := path.Ext(name)
			nameWithoutExt := strings.TrimSuffix(name, ext)
			name = fmt.Sprintf("%s (%d)%s", nameWithoutExt, count, ext)
		}
		seenNames[cf.Name]++ // Increment count for the original name

		// Clean and join path
		fullPath := path.Join(virtualPath, name)
		if !strings.HasPrefix(fullPath, "/") {
			fullPath = "/" + fullPath
		}

		status := database.FileStatusSynced

		existing, exists := dbFileMap[cf.Name] // Note: this logic is slightly flawed if we renamed, but dbFileMap is keyed by original db name.
		// If we renamed the file to "file (1).txt", we shouldn't look up "file.txt" in dbFileMap for *this* iteration if it was already matched.
		// However, dbFileMap logic above matches by EXACT virtual path name.

		// Because we're iterating cloud files, if we encounter a duplicate, we generate a NEW name.
		// We should check if that NEW name exists in DB.

		// Let's refine the lookup.
		// Actually, simpler approach: Just check if fullPath exists in dbFileMap?
		// But dbFileMap is keyed by *filename*.
		// If we have "test.txt" and "test (1).txt" in DB.
		// Cloud has "test.txt"[ID1] and "test.txt"[ID2].
		// Iteration 1: "test.txt". seen=0. Name="test.txt". Lookup "test.txt". Found. Update.
		// Iteration 2: "test.txt". seen=1. Name="test (1).txt". Lookup "test (1).txt". Found? Update.

		// We need to re-key dbFileMap or handled it carefully.

		existingMatch, exists := dbFileMap[name]

		if exists && existingMatch.CloudFileID == cf.ID {
			// Exact match by name AND ID - perfect update
			existing = existingMatch
		} else if exists {
			// Name collision in DB.
			// If IDs mismatch, it might be a different file taking this slot.
			// Or the same file that's already there.
			// Let's rely on ID match preferentially? No, DB doesn't index by ID here, only by path.
			// If "test (1).txt" exists in DB and corresponds to this CloudID, we update it.
			existing = existingMatch
		} else {
			// Not found by name.
			// It might be a new file or a rename.
			exists = false
		}

		if exists {
			// Check if update needed
			updateNeeded := false
			if existing.CloudFileID != cf.ID {
				existing.CloudFileID = cf.ID // ID changed? rare but possible if replaced
				updateNeeded = true
			}
			if existing.SizeBytes != cf.Size {
				existing.SizeBytes = cf.Size
				updateNeeded = true
			}
			// MimeType check?

			if updateNeeded {
				existing.UpdatedAt = time.Now()
				if err := e.db.UpdateFile(ctx, existing); err != nil {
					log.Printf("Failed to update file %s: %v", fullPath, err)
				}
			}
			// Remove from map so we know it's handled
			delete(dbFileMap, name)

			// If it's a directory, we must recurse regardless of update
			if cf.IsDir {
				if err := e.syncFolder(ctx, provider, providerID, cf.ID, fullPath); err != nil {
					log.Printf("Failed to sync folder %s: %v", fullPath, err)
				}
			}
		} else {
			// Insert new file
			// Default new files to pinned=false
			newFile := &database.File{
				VirtualPath: fullPath,
				ProviderID:  providerID,
				CloudFileID: cf.ID,
				CloudPath:   fullPath,
				SizeBytes:   cf.Size,
				MimeType:    cf.MimeType,
				Status:      status,
				IsDir:       cf.IsDir,
				Pinned:      false,
			}
			if err := e.db.CreateFile(ctx, newFile); err != nil {
				log.Printf("Failed to create file %s: %v", fullPath, err)
			}

			// Recurse if directory
			if cf.IsDir {
				if err := e.syncFolder(ctx, provider, providerID, cf.ID, fullPath); err != nil {
					log.Printf("Failed to sync folder %s: %v", fullPath, err)
				}
			}
		}
	}

	// Handle deletions (remaining in dbFileMap)
	for _, f := range dbFileMap {
		// Only delete if it thinks it's synced. If it's uploading/pending, don't delete!
		if f.Status == database.FileStatusSynced || f.Status == database.FileStatusError {
			log.Printf("Deleting file %s (remote missing)", f.VirtualPath)
			if err := e.db.DeleteFile(ctx, f.ID); err != nil {
				log.Printf("Failed to delete file %s: %v", f.VirtualPath, err)
			}
		}
	}

	return nil
}
