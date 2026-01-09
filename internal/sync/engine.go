package sync

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"cloudunify/internal/database"
	"cloudunify/internal/providers"
	"cloudunify/internal/storage"
)

// Engine coordinates sync operations between local filesystem and cloud providers
type Engine struct {
	db           *database.DB
	queue        *Queue
	allocator    *storage.Allocator
	providers    map[int64]providers.CloudProvider
	providersMu  sync.RWMutex

	uploadWorkers   int
	downloadWorkers int

	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	running    bool
	runningMu  sync.Mutex
}

// NewEngine creates a new sync engine
func NewEngine(db *database.DB, allocator *storage.Allocator, uploadWorkers, downloadWorkers int) *Engine {
	return &Engine{
		db:              db,
		queue:           NewQueue(db),
		allocator:       allocator,
		providers:       make(map[int64]providers.CloudProvider),
		uploadWorkers:   uploadWorkers,
		downloadWorkers: downloadWorkers,
	}
}

// Queue returns the sync queue
func (e *Engine) Queue() *Queue {
	return e.queue
}

// RegisterProvider registers a cloud provider instance
func (e *Engine) RegisterProvider(providerID int64, provider providers.CloudProvider) {
	e.providersMu.Lock()
	defer e.providersMu.Unlock()
	e.providers[providerID] = provider
}

// UnregisterProvider removes a cloud provider instance
func (e *Engine) UnregisterProvider(providerID int64) {
	e.providersMu.Lock()
	defer e.providersMu.Unlock()
	delete(e.providers, providerID)
}

// GetProvider retrieves a registered provider
func (e *Engine) GetProvider(providerID int64) (providers.CloudProvider, bool) {
	e.providersMu.RLock()
	defer e.providersMu.RUnlock()
	p, ok := e.providers[providerID]
	return p, ok
}

// Start begins processing the sync queue
func (e *Engine) Start(ctx context.Context) error {
	e.runningMu.Lock()
	if e.running {
		e.runningMu.Unlock()
		return fmt.Errorf("sync engine already running")
	}
	e.running = true
	e.runningMu.Unlock()

	e.ctx, e.cancel = context.WithCancel(ctx)

	// Start upload workers
	for i := 0; i < e.uploadWorkers; i++ {
		e.wg.Add(1)
		go e.uploadWorker(i)
	}

	// Start download workers
	for i := 0; i < e.downloadWorkers; i++ {
		e.wg.Add(1)
		go e.downloadWorker(i)
	}

	log.Printf("Sync engine started with %d upload workers and %d download workers",
		e.uploadWorkers, e.downloadWorkers)

	return nil
}

// Stop gracefully stops the sync engine
func (e *Engine) Stop() {
	e.runningMu.Lock()
	if !e.running {
		e.runningMu.Unlock()
		return
	}
	e.running = false
	e.runningMu.Unlock()

	e.cancel()
	e.wg.Wait()
	log.Println("Sync engine stopped")
}

// uploadWorker processes upload jobs from the queue
func (e *Engine) uploadWorker(id int) {
	defer e.wg.Done()
	log.Printf("Upload worker %d started", id)

	for {
		select {
		case <-e.ctx.Done():
			log.Printf("Upload worker %d stopping", id)
			return
		default:
			item, err := e.queue.DequeueByOperation(e.ctx, string(database.SyncOpUpload))
			if err != nil {
				log.Printf("Upload worker %d error: %v", id, err)
				time.Sleep(time.Second)
				continue
			}

			if item == nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}

			e.processUpload(item)
		}
	}
}

// downloadWorker processes download jobs from the queue
func (e *Engine) downloadWorker(id int) {
	defer e.wg.Done()
	log.Printf("Download worker %d started", id)

	for {
		select {
		case <-e.ctx.Done():
			log.Printf("Download worker %d stopping", id)
			return
		default:
			// Download workers handle download and delete operations
			item, err := e.queue.DequeueByOperation(e.ctx, string(database.SyncOpDownload))
			if err != nil {
				log.Printf("Download worker %d error: %v", id, err)
				time.Sleep(time.Second)
				continue
			}

			if item == nil {
				// Also check for delete operations
				item, err = e.queue.DequeueByOperation(e.ctx, string(database.SyncOpDelete))
				if err != nil {
					log.Printf("Download worker %d error: %v", id, err)
					time.Sleep(time.Second)
					continue
				}
			}

			if item == nil {
				time.Sleep(500 * time.Millisecond)
				continue
			}

			switch item.Operation {
			case database.SyncOpDownload:
				e.processDownload(item)
			case database.SyncOpDelete:
				e.processDelete(item)
			}
		}
	}
}

// processUpload handles a single upload job
func (e *Engine) processUpload(item *database.SyncQueueItem) {
	log.Printf("Processing upload: %s", item.VirtualPath)

	// Get file info
	fileInfo, err := os.Stat(item.LocalPath)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to stat file: %v", err))
		return
	}

	// Choose provider if not specified
	var providerID int64
	if item.ProviderID.Valid {
		providerID = item.ProviderID.Int64
	} else {
		provider, err := e.allocator.ChooseProvider(e.ctx, fileInfo.Size())
		if err != nil {
			e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to choose provider: %v", err))
			return
		}
		providerID = provider.ID
	}

	// Get provider instance
	provider, ok := e.GetProvider(providerID)
	if !ok {
		e.queue.Fail(e.ctx, item.ID, "provider not registered")
		return
	}

	// Open file
	file, err := os.Open(item.LocalPath)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to open file: %v", err))
		return
	}
	defer file.Close()

	// Create progress reader
	progressReader := providers.NewProgressReader(file, fileInfo.Size(), func(transferred, total int64) {
		progress := int(float64(transferred) / float64(total) * 100)
		e.queue.UpdateProgress(e.ctx, item.ID, progress)
	})

	// Upload to cloud
	metadata, err := provider.UploadStream(e.ctx, progressReader, item.VirtualPath, fileInfo.Size())
	if err != nil {
		if providers.IsRetriableError(err) && item.RetryCount < 3 {
			e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("upload failed (will retry): %v", err))
			e.queue.Retry(e.ctx, item.ID)
		} else {
			e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("upload failed: %v", err))
		}
		return
	}

	// Create file record in database
	dbFile := &database.File{
		VirtualPath: item.VirtualPath,
		ProviderID:  providerID,
		CloudFileID: metadata.ID,
		CloudPath:   metadata.Path,
		SizeBytes:   metadata.Size,
		Checksum:    metadata.Checksum,
		MimeType:    metadata.MimeType,
		Status:      database.FileStatusSynced,
	}

	log.Printf("Upload metadata: ID=%s, Size=%d, MimeType=%s", metadata.ID, metadata.Size, metadata.MimeType)

	// Try to create, or update if file already exists
	if err := e.db.CreateFile(e.ctx, dbFile); err != nil {
		// Check if file already exists
		existingFile, _ := e.db.GetFileByPath(e.ctx, item.VirtualPath)
		if existingFile != nil {
			// Update existing record
			existingFile.CloudFileID = metadata.ID
			existingFile.CloudPath = metadata.Path
			existingFile.SizeBytes = metadata.Size
			existingFile.Checksum = metadata.Checksum
			existingFile.MimeType = metadata.MimeType
			existingFile.Status = database.FileStatusSynced
			log.Printf("Updating existing file record: %s with size %d", existingFile.VirtualPath, existingFile.SizeBytes)
			if err := e.db.UpdateFile(e.ctx, existingFile); err != nil {
				log.Printf("Warning: failed to update file record: %v", err)
			}
		} else {
			log.Printf("Warning: failed to create file record: %v", err)
		}
	}

	// Update provider usage
	e.allocator.UpdateUsage(e.ctx, providerID, metadata.Size)

	// Mark job as complete
	e.queue.Complete(e.ctx, item.ID)
	log.Printf("Upload complete: %s", item.VirtualPath)
}

// processDownload handles a single download job
func (e *Engine) processDownload(item *database.SyncQueueItem) {
	log.Printf("Processing download: %s", item.VirtualPath)

	// Get file record
	file, err := e.db.GetFileByPath(e.ctx, item.VirtualPath)
	if err != nil || file == nil {
		e.queue.Fail(e.ctx, item.ID, "file not found in database")
		return
	}

	// Get provider instance
	provider, ok := e.GetProvider(file.ProviderID)
	if !ok {
		e.queue.Fail(e.ctx, item.ID, "provider not registered")
		return
	}

	// Create local file
	localPath := item.LocalPath
	if localPath == "" {
		localPath = filepath.Join(os.TempDir(), filepath.Base(item.VirtualPath))
	}

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to create directory: %v", err))
		return
	}

	outFile, err := os.Create(localPath)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to create file: %v", err))
		return
	}
	defer outFile.Close()

	// Download from cloud
	stream, err := provider.DownloadStream(e.ctx, file.CloudFileID)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to start download: %v", err))
		return
	}
	defer stream.Close()

	// Copy with progress tracking
	written, err := io.Copy(outFile, stream)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("download failed: %v", err))
		return
	}

	log.Printf("Downloaded %d bytes for %s", written, item.VirtualPath)
	e.queue.Complete(e.ctx, item.ID)
}

// processDelete handles a single delete job
func (e *Engine) processDelete(item *database.SyncQueueItem) {
	log.Printf("Processing delete: %s", item.VirtualPath)

	// Get file record
	file, err := e.db.GetFileByPath(e.ctx, item.VirtualPath)
	if err != nil {
		e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("failed to get file: %v", err))
		return
	}

	if file == nil {
		// File doesn't exist, consider it deleted
		e.queue.Complete(e.ctx, item.ID)
		return
	}

	// Get provider instance
	provider, ok := e.GetProvider(file.ProviderID)
	if !ok {
		e.queue.Fail(e.ctx, item.ID, "provider not registered")
		return
	}

	// Delete from cloud
	if err := provider.Delete(e.ctx, file.CloudFileID); err != nil {
		if providers.IsRetriableError(err) && item.RetryCount < 3 {
			e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("delete failed (will retry): %v", err))
			e.queue.Retry(e.ctx, item.ID)
		} else {
			e.queue.Fail(e.ctx, item.ID, fmt.Sprintf("delete failed: %v", err))
		}
		return
	}

	// Update provider usage
	e.allocator.UpdateUsage(e.ctx, file.ProviderID, -file.SizeBytes)

	// Delete file record
	if err := e.db.DeleteFile(e.ctx, file.ID); err != nil {
		log.Printf("Warning: failed to delete file record: %v", err)
	}

	e.queue.Complete(e.ctx, item.ID)
	log.Printf("Delete complete: %s", item.VirtualPath)
}

// EnqueueUpload adds an upload job to the queue
func (e *Engine) EnqueueUpload(ctx context.Context, virtualPath, localPath string, priority int) (*database.SyncQueueItem, error) {
	return e.queue.Enqueue(ctx, database.SyncOpUpload, virtualPath, localPath, nil, priority)
}

// EnqueueDownload adds a download job to the queue
func (e *Engine) EnqueueDownload(ctx context.Context, virtualPath, localPath string, priority int) (*database.SyncQueueItem, error) {
	return e.queue.Enqueue(ctx, database.SyncOpDownload, virtualPath, localPath, nil, priority)
}

// EnqueueDelete adds a delete job to the queue
func (e *Engine) EnqueueDelete(ctx context.Context, virtualPath string, priority int) (*database.SyncQueueItem, error) {
	return e.queue.Enqueue(ctx, database.SyncOpDelete, virtualPath, "", nil, priority)
}

// IsRunning returns whether the engine is running
func (e *Engine) IsRunning() bool {
	e.runningMu.Lock()
	defer e.runningMu.Unlock()
	return e.running
}
