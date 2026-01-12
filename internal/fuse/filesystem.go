package fuse

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"

	"cloudunify/internal/database"
	"cloudunify/internal/providers"
	"cloudunify/internal/sync"
)

// PathInfo contains parsed path information for routing
type PathInfo struct {
	IsRoot           bool   // Path is "/"
	IsProvidersDir   bool   // Path is "/.providers"
	IsProviderRoot   bool   // Path is "/.providers/<provider>" or "/<provider>"
	ProviderName     string // The provider display name (e.g., "Google Drive", "OneDrive", "iCloud")
	ProviderID       int64  // The provider database ID (0 if unknown/not looked up)
	RelativePath     string // Path relative to provider root (e.g., "/Documents/file.txt") - DEPRECATED
	VirtualPath      string // Full namespaced virtual path for DB lookup (e.g., "/OneDrive/Documents/file.txt")
	InProvidersNS    bool   // Whether path is under /.providers/
	OriginalPath     string // The original full path
}

// parsePath analyzes a FUSE path and returns routing information
func (fs *CloudUnifyFS) parsePath(path string) *PathInfo {
	info := &PathInfo{OriginalPath: path}

	// Root directory
	if path == "/" {
		info.IsRoot = true
		return info
	}

	// Clean and split path
	cleanPath := strings.TrimPrefix(path, "/")
	parts := strings.SplitN(cleanPath, "/", 2)
	firstPart := parts[0]

	// Check for .providers namespace
	if firstPart == ".providers" {
		info.InProvidersNS = true

		if len(parts) == 1 {
			// Just "/.providers"
			info.IsProvidersDir = true
			return info
		}

		// Parse provider name from second part
		subPath := parts[1]
		subParts := strings.SplitN(subPath, "/", 2)
		info.ProviderName = subParts[0]

		if len(subParts) == 1 {
			// "/.providers/<provider>"
			info.IsProviderRoot = true
			info.RelativePath = "/"
			info.VirtualPath = "/" + info.ProviderName
		} else {
			// "/.providers/<provider>/<path>"
			info.RelativePath = "/" + subParts[1]
			info.VirtualPath = "/" + info.ProviderName + "/" + subParts[1]
		}

		return info
	}

	// Check if first part is a known provider name
	knownProviders := database.AllProviderDisplayNames()
	for _, pn := range knownProviders {
		if firstPart == pn {
			info.ProviderName = pn
			info.InProvidersNS = false // In merged namespace but provider-prefixed

			if len(parts) == 1 {
				// "/<provider>"
				info.IsProviderRoot = true
				info.RelativePath = "/"
				info.VirtualPath = "/" + pn
			} else {
				// "/<provider>/<path>"
				info.RelativePath = "/" + parts[1]
				info.VirtualPath = "/" + pn + "/" + parts[1]
			}

			return info
		}
	}

	// Not a provider path - treat as merged namespace path
	// This would be for files at the root of merged view
	info.RelativePath = "/" + cleanPath
	info.VirtualPath = "/" + cleanPath
	return info
}

// getProviderID looks up the provider ID for a given provider display name
func (fs *CloudUnifyFS) getProviderID(providerName string) (int64, error) {
	providerType := database.ProviderTypeFromDisplayName(providerName)
	provider, err := fs.db.GetProviderByType(fs.ctx, providerType)
	if err != nil {
		return 0, err
	}
	if provider == nil {
		return 0, nil
	}
	return provider.ID, nil
}

// DownloadProgressCallback is called with download progress updates
type DownloadProgressCallback func(virtualPath string, downloaded, total int64, status string)

// CloudUnifyFS implements the FUSE filesystem interface
type CloudUnifyFS struct {
	fuse.FileSystemBase

	db         *database.DB
	syncEngine *sync.Engine
	mountPath  string
	stagingDir string
	cacheDir   string

	// Cache manager for downloaded files
	cacheManager *CacheManager

	// Progress callback for download updates
	progressCallback DownloadProgressCallback

	// Download timeout (default 30s)
	downloadTimeout time.Duration

	// File handles
	handles   map[uint64]*FileHandle
	handlesMu gosync.RWMutex
	nextFH    uint64

	// Pending files (files being written but not yet uploaded)
	pendingFiles   map[string]*PendingFile
	pendingFilesMu gosync.RWMutex

	// Active downloads tracking
	activeDownloads   map[string]chan struct{}
	activeDownloadsMu gosync.Mutex

	// Context for operations
	ctx    context.Context
	cancel context.CancelFunc
}

// PendingFile represents a file being created
type PendingFile struct {
	Path        string
	StagingPath string
	Size        int64
	CreatedAt   time.Time
	Queued      bool // Whether upload has been queued
}

// FileHandle represents an open file
type FileHandle struct {
	Path      string
	File      *os.File // For staged files during write
	ReadOnly  bool
	FileEntry *database.File
}

// NewCloudUnifyFS creates a new CloudUnify filesystem
func NewCloudUnifyFS(db *database.DB, syncEngine *sync.Engine, mountPath, stagingDir, cacheDir string) *CloudUnifyFS {
	ctx, cancel := context.WithCancel(context.Background())

	// Create cache manager with 10GB default limit
	cacheManager := NewCacheManager(db, cacheDir, 10)

	return &CloudUnifyFS{
		db:              db,
		syncEngine:      syncEngine,
		mountPath:       mountPath,
		stagingDir:      stagingDir,
		cacheDir:        cacheDir,
		cacheManager:    cacheManager,
		downloadTimeout: 30 * time.Second,
		handles:         make(map[uint64]*FileHandle),
		pendingFiles:    make(map[string]*PendingFile),
		activeDownloads: make(map[string]chan struct{}),
		nextFH:          1,
		ctx:             ctx,
		cancel:          cancel,
	}
}

// SetProgressCallback sets the callback for download progress updates
func (fs *CloudUnifyFS) SetProgressCallback(callback DownloadProgressCallback) {
	fs.progressCallback = callback
}

// SetDownloadTimeout sets the download timeout
func (fs *CloudUnifyFS) SetDownloadTimeout(timeout time.Duration) {
	fs.downloadTimeout = timeout
}

// Init is called when the filesystem is mounted
func (fs *CloudUnifyFS) Init() {
	log.Printf("CloudUnify filesystem initialized at %s", fs.mountPath)
}

// Destroy is called when the filesystem is unmounted
func (fs *CloudUnifyFS) Destroy() {
	fs.cancel()
	log.Println("CloudUnify filesystem destroyed")
}

// Statfs returns filesystem statistics
func (fs *CloudUnifyFS) Statfs(path string, stat *fuse.Statfs_t) int {
	// Get total storage from all providers
	summary, err := fs.db.GetStorageSummary(fs.ctx)
	if err != nil {
		return -fuse.EIO
	}

	// Block size of 4KB
	blockSize := int64(4096)

	stat.Bsize = uint64(blockSize)
	stat.Frsize = uint64(blockSize)
	stat.Blocks = uint64(summary.TotalBytes / blockSize)
	stat.Bfree = uint64(summary.FreeBytes / blockSize)
	stat.Bavail = uint64(summary.FreeBytes / blockSize)
	stat.Files = 1000000 // Max files
	stat.Ffree = 999999
	stat.Favail = 999999
	stat.Namemax = 255

	return 0
}

// Getattr returns file attributes
func (fs *CloudUnifyFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
	pathInfo := fs.parsePath(path)
	now := time.Now()

	// Helper to set directory attributes
	setDirAttr := func() {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
		stat.Uid = uint32(os.Getuid())
		stat.Gid = uint32(os.Getgid())
		stat.Atim = fuse.NewTimespec(now)
		stat.Mtim = fuse.NewTimespec(now)
		stat.Ctim = fuse.NewTimespec(now)
	}

	// Root directory
	if pathInfo.IsRoot {
		setDirAttr()
		return 0
	}

	// .providers directory
	if pathInfo.IsProvidersDir {
		setDirAttr()
		return 0
	}

	// Provider root directories (e.g., /.providers/OneDrive or /OneDrive)
	if pathInfo.IsProviderRoot {
		// Check if it's a known provider
		knownProviders := database.AllProviderDisplayNames()
		for _, pn := range knownProviders {
			if pn == pathInfo.ProviderName {
				setDirAttr()
				return 0
			}
		}
		return -fuse.ENOENT
	}

	// For paths with a provider, look up the file in that provider's namespace
	if pathInfo.ProviderName != "" {
		providerID, err := fs.getProviderID(pathInfo.ProviderName)
		if err != nil {
			return -fuse.EIO
		}
		if providerID == 0 {
			return -fuse.ENOENT
		}

		// Check for pending files first (files being written)
		fs.pendingFilesMu.RLock()
		pending, isPending := fs.pendingFiles[path]
		fs.pendingFilesMu.RUnlock()

		if isPending {
			if info, err := os.Stat(pending.StagingPath); err == nil {
				stat.Mode = fuse.S_IFREG | 0644
				stat.Nlink = 1
				stat.Size = info.Size()
				stat.Uid = uint32(os.Getuid())
				stat.Gid = uint32(os.Getgid())
				stat.Atim = fuse.NewTimespec(info.ModTime())
				stat.Mtim = fuse.NewTimespec(info.ModTime())
				stat.Ctim = fuse.NewTimespec(pending.CreatedAt)
				return 0
			}
		}

		// Check staging directory
		stagingPath := fs.getStagingPath(path)
		if info, err := os.Stat(stagingPath); err == nil {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Nlink = 1
			stat.Size = info.Size()
			stat.Uid = uint32(os.Getuid())
			stat.Gid = uint32(os.Getgid())
			stat.Atim = fuse.NewTimespec(info.ModTime())
			stat.Mtim = fuse.NewTimespec(info.ModTime())
			stat.Ctim = fuse.NewTimespec(info.ModTime())
			return 0
		}

		// Look up file in database by virtual path (includes provider namespace)
		file, err := fs.db.GetFileByPath(fs.ctx, pathInfo.VirtualPath)
		if err != nil {
			return -fuse.EIO
		}

		if file != nil {
			if file.IsDir {
				stat.Mode = fuse.S_IFDIR | 0755
				stat.Nlink = 2
			} else {
				stat.Mode = fuse.S_IFREG | 0644
				stat.Nlink = 1
				stat.Size = file.SizeBytes
			}

			stat.Uid = uint32(os.Getuid())
			stat.Gid = uint32(os.Getgid())
			stat.Atim = fuse.NewTimespec(file.UpdatedAt)
			stat.Mtim = fuse.NewTimespec(file.UpdatedAt)
			stat.Ctim = fuse.NewTimespec(file.CreatedAt)
			return 0
		}

		return -fuse.ENOENT
	}

	// Fallback: Check for pending files first (files being written)
	fs.pendingFilesMu.RLock()
	pending, isPending := fs.pendingFiles[path]
	fs.pendingFilesMu.RUnlock()

	if isPending {
		// Get size from staging file
		if info, err := os.Stat(pending.StagingPath); err == nil {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Nlink = 1
			stat.Size = info.Size()
			stat.Uid = uint32(os.Getuid())
			stat.Gid = uint32(os.Getgid())
			stat.Atim = fuse.NewTimespec(info.ModTime())
			stat.Mtim = fuse.NewTimespec(info.ModTime())
			stat.Ctim = fuse.NewTimespec(pending.CreatedAt)
			return 0
		}
	}

	// Check staging directory first - prefer local file size over database
	// This ensures recently uploaded files show correct size
	stagingPath := fs.getStagingPath(path)
	if info, err := os.Stat(stagingPath); err == nil {
		stat.Mode = fuse.S_IFREG | 0644
		stat.Nlink = 1
		stat.Size = info.Size()
		stat.Uid = uint32(os.Getuid())
		stat.Gid = uint32(os.Getgid())
		stat.Atim = fuse.NewTimespec(info.ModTime())
		stat.Mtim = fuse.NewTimespec(info.ModTime())
		stat.Ctim = fuse.NewTimespec(info.ModTime())
		return 0
	}

	// Look up file in database (legacy path support)
	file, err := fs.db.GetFileByPath(fs.ctx, path)
	if err != nil {
		return -fuse.EIO
	}

	if file != nil {
		if file.IsDir {
			stat.Mode = fuse.S_IFDIR | 0755
			stat.Nlink = 2
		} else {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Nlink = 1
			stat.Size = file.SizeBytes
		}

		stat.Uid = uint32(os.Getuid())
		stat.Gid = uint32(os.Getgid())
		stat.Atim = fuse.NewTimespec(file.UpdatedAt)
		stat.Mtim = fuse.NewTimespec(file.UpdatedAt)
		stat.Ctim = fuse.NewTimespec(file.CreatedAt)
		return 0
	}

	return -fuse.ENOENT
}

// Readdir reads directory contents
func (fs *CloudUnifyFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	fill(".", nil, 0)
	fill("..", nil, 0)

	pathInfo := fs.parsePath(path)

	// Root directory: show provider directories and .providers
	if pathInfo.IsRoot {
		var stat fuse.Stat_t
		stat.Mode = fuse.S_IFDIR | 0755

		// Show .providers directory
		if !fill(".providers", &stat, 0) {
			return 0
		}

		// Show all known provider directories
		for _, providerName := range database.AllProviderDisplayNames() {
			if !fill(providerName, &stat, 0) {
				return 0
			}
		}
		return 0
	}

	// .providers directory: show all provider directories
	if pathInfo.IsProvidersDir {
		var stat fuse.Stat_t
		stat.Mode = fuse.S_IFDIR | 0755

		for _, providerName := range database.AllProviderDisplayNames() {
			if !fill(providerName, &stat, 0) {
				return 0
			}
		}
		return 0
	}

	// Provider directory (either /.providers/<provider> or /<provider>)
	if pathInfo.ProviderName != "" {
		providerID, err := fs.getProviderID(pathInfo.ProviderName)
		if err != nil {
			log.Printf("Readdir error getting provider ID: %v", err)
			return -fuse.EIO
		}

		// If provider doesn't exist yet (not authenticated), show empty directory
		if providerID == 0 {
			return 0
		}

		// Track names we've seen
		seenNames := make(map[string]bool)

		// List files in this provider's directory from database
		// Use VirtualPath which includes the provider namespace (e.g., "/OneDrive" or "/OneDrive/Documents")
		files, err := fs.db.ListFilesInDirectory(fs.ctx, pathInfo.VirtualPath)
		if err != nil {
			log.Printf("Readdir error listing provider files: %v", err)
			return -fuse.EIO
		}

		for _, file := range files {
			name := filepath.Base(file.VirtualPath)
			seenNames[name] = true
			var stat fuse.Stat_t

			if file.IsDir {
				stat.Mode = fuse.S_IFDIR | 0755
			} else {
				stat.Mode = fuse.S_IFREG | 0644
				stat.Size = file.SizeBytes
			}

			if !fill(name, &stat, 0) {
				break
			}
		}

		// Also include pending files for this path
		fs.pendingFilesMu.RLock()
		for pendingPath, pending := range fs.pendingFiles {
			pendingDir := filepath.Dir(pendingPath)
			if pendingDir == path {
				name := filepath.Base(pendingPath)
				if !seenNames[name] {
					seenNames[name] = true
					var stat fuse.Stat_t
					stat.Mode = fuse.S_IFREG | 0644
					stat.Size = pending.Size
					if !fill(name, &stat, 0) {
						break
					}
				}
			}
		}
		fs.pendingFilesMu.RUnlock()

		// Also include files from staging directory
		stagingDir := fs.getStagingPath(path)
		if entries, err := os.ReadDir(stagingDir); err == nil {
			for _, entry := range entries {
				name := entry.Name()
				if !seenNames[name] && !strings.HasPrefix(name, ".") {
					seenNames[name] = true
					var stat fuse.Stat_t
					if entry.IsDir() {
						stat.Mode = fuse.S_IFDIR | 0755
					} else {
						stat.Mode = fuse.S_IFREG | 0644
						if info, err := entry.Info(); err == nil {
							stat.Size = info.Size()
						}
					}
					if !fill(name, &stat, 0) {
						break
					}
				}
			}
		}

		return 0
	}

	// Fallback for legacy paths (files directly in database with old-style paths)
	seenNames := make(map[string]bool)

	// List files in this directory from database
	files, err := fs.db.ListFilesInDirectory(fs.ctx, path)
	if err != nil {
		log.Printf("Readdir error: %v", err)
		return -fuse.EIO
	}

	for _, file := range files {
		name := filepath.Base(file.VirtualPath)
		seenNames[name] = true
		var stat fuse.Stat_t

		if file.IsDir {
			stat.Mode = fuse.S_IFDIR | 0755
		} else {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Size = file.SizeBytes
		}

		if !fill(name, &stat, 0) {
			break
		}
	}

	// Also include pending files
	fs.pendingFilesMu.RLock()
	for pendingPath, pending := range fs.pendingFiles {
		pendingDir := filepath.Dir(pendingPath)
		if pendingDir == path || (path == "/" && pendingDir == ".") {
			name := filepath.Base(pendingPath)
			if !seenNames[name] {
				seenNames[name] = true
				var stat fuse.Stat_t
				stat.Mode = fuse.S_IFREG | 0644
				stat.Size = pending.Size
				if !fill(name, &stat, 0) {
					break
				}
			}
		}
	}
	fs.pendingFilesMu.RUnlock()

	// Also include files from staging directory
	stagingDir := fs.getStagingPath(path)
	if entries, err := os.ReadDir(stagingDir); err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !seenNames[name] && !strings.HasPrefix(name, ".") {
				seenNames[name] = true
				var stat fuse.Stat_t
				if entry.IsDir() {
					stat.Mode = fuse.S_IFDIR | 0755
				} else {
					stat.Mode = fuse.S_IFREG | 0644
					if info, err := entry.Info(); err == nil {
						stat.Size = info.Size()
					}
				}
				if !fill(name, &stat, 0) {
					break
				}
			}
		}
	}

	return 0
}

// Mkdir creates a directory
func (fs *CloudUnifyFS) Mkdir(path string, mode uint32) int {
	// Create directory entry in database
	file := &database.File{
		VirtualPath: path,
		ProviderID:  0, // Will be set when files are added
		CloudFileID: "",
		SizeBytes:   0,
		Status:      database.FileStatusSynced,
		IsDir:       true,
	}

	if err := fs.db.CreateFile(fs.ctx, file); err != nil {
		log.Printf("Mkdir error: %v", err)
		return -fuse.EIO
	}

	return 0
}

// Rmdir removes a directory
func (fs *CloudUnifyFS) Rmdir(path string) int {
	// Check if directory is empty
	files, err := fs.db.ListFilesInDirectory(fs.ctx, path)
	if err != nil {
		return -fuse.EIO
	}

	if len(files) > 0 {
		return -fuse.ENOTEMPTY
	}

	if err := fs.db.DeleteFileByPath(fs.ctx, path); err != nil {
		return -fuse.EIO
	}

	return 0
}

// Create creates a new file
func (fs *CloudUnifyFS) Create(path string, flags int, mode uint32) (int, uint64) {
	// Create staging file
	stagingPath := fs.getStagingPath(path)
	if err := os.MkdirAll(filepath.Dir(stagingPath), 0755); err != nil {
		log.Printf("Create error: %v", err)
		return -fuse.EIO, 0
	}

	file, err := os.Create(stagingPath)
	if err != nil {
		log.Printf("Create error: %v", err)
		return -fuse.EIO, 0
	}

	// Track as pending file
	fs.pendingFilesMu.Lock()
	fs.pendingFiles[path] = &PendingFile{
		Path:        path,
		StagingPath: stagingPath,
		Size:        0,
		CreatedAt:   time.Now(),
	}
	fs.pendingFilesMu.Unlock()

	// Allocate file handle
	fh := fs.allocHandle(&FileHandle{
		Path:     path,
		File:     file,
		ReadOnly: false,
	})

	return 0, fh
}

// downloadToCache downloads a file from cloud to cache synchronously
// Returns the local cache path and any error
func (fs *CloudUnifyFS) downloadToCache(file *database.File) (string, error) {
	// Check if another download is already in progress for this file
	fs.activeDownloadsMu.Lock()
	if doneChan, exists := fs.activeDownloads[file.VirtualPath]; exists {
		fs.activeDownloadsMu.Unlock()
		// Wait for the existing download to complete
		<-doneChan
		// Check if file is now cached
		cachePath, cached, err := fs.cacheManager.GetCachedPath(fs.ctx, file.ID)
		if err != nil {
			return "", err
		}
		if cached {
			return cachePath, nil
		}
		return "", fmt.Errorf("download completed but file not in cache")
	}

	// Mark this download as in progress
	doneChan := make(chan struct{})
	fs.activeDownloads[file.VirtualPath] = doneChan
	fs.activeDownloadsMu.Unlock()

	defer func() {
		fs.activeDownloadsMu.Lock()
		delete(fs.activeDownloads, file.VirtualPath)
		close(doneChan)
		fs.activeDownloadsMu.Unlock()
	}()

	// Get the provider
	provider, ok := fs.syncEngine.GetProvider(file.ProviderID)
	if !ok {
		return "", fmt.Errorf("provider %d not registered", file.ProviderID)
	}

	// Create cache directory and file
	cachePath := fs.cacheManager.GetCachePath(file.VirtualPath)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	cacheFile, err := os.Create(cachePath)
	if err != nil {
		return "", fmt.Errorf("failed to create cache file: %w", err)
	}
	defer cacheFile.Close()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(fs.ctx, fs.downloadTimeout)
	defer cancel()

	// Notify progress: starting
	if fs.progressCallback != nil {
		fs.progressCallback(file.VirtualPath, 0, file.SizeBytes, "downloading")
	}

	// Download to cache file
	if gdProvider, ok := provider.(*providers.GoogleDriveProvider); ok {
		err = gdProvider.DownloadWithProgress(ctx, file.CloudFileID, cacheFile, file.SizeBytes,
			func(downloaded, total int64) {
				if fs.progressCallback != nil {
					fs.progressCallback(file.VirtualPath, downloaded, total, "downloading")
				}
			})
	} else {
		// Fallback to simple streaming download
		stream, streamErr := provider.DownloadStream(ctx, file.CloudFileID)
		if streamErr != nil {
			os.Remove(cachePath)
			return "", fmt.Errorf("failed to start download: %w", streamErr)
		}
		defer stream.Close()

		var written int64
		buf := make([]byte, 32*1024)
		for {
			n, readErr := stream.Read(buf)
			if n > 0 {
				w, writeErr := cacheFile.Write(buf[:n])
				if writeErr != nil {
					os.Remove(cachePath)
					return "", fmt.Errorf("failed to write to cache: %w", writeErr)
				}
				written += int64(w)
				if fs.progressCallback != nil && file.SizeBytes > 0 {
					fs.progressCallback(file.VirtualPath, written, file.SizeBytes, "downloading")
				}
			}
			if readErr != nil {
				// Check for EOF (successful end of stream)
				if readErr == io.EOF {
					break
				}
				// Actual error - fail the download
				os.Remove(cachePath)
				return "", fmt.Errorf("failed to read from stream: %w", readErr)
			}
		}
	}

	if err != nil {
		os.Remove(cachePath)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("download timeout after %v", fs.downloadTimeout)
		}
		return "", fmt.Errorf("download failed: %w", err)
	}

	// Add to cache database
	if err := fs.cacheManager.AddToCache(fs.ctx, file.ID, cachePath); err != nil {
		log.Printf("Warning: failed to add file to cache database: %v", err)
		// Continue anyway, file is still accessible
	}

	// Notify progress: complete
	if fs.progressCallback != nil {
		fs.progressCallback(file.VirtualPath, file.SizeBytes, file.SizeBytes, "completed")
	}

	return cachePath, nil
}

// Open opens a file
func (fs *CloudUnifyFS) Open(path string, flags int) (int, uint64) {
	pathInfo := fs.parsePath(path)

	// Check if this is a write operation
	isWrite := (flags & (fuse.O_WRONLY | fuse.O_RDWR | fuse.O_CREAT | fuse.O_TRUNC)) != 0

	// First check if file is in staging directory (recently uploaded or pending)
	stagingPath := fs.getStagingPath(path)
	if _, err := os.Stat(stagingPath); err == nil {
		var f *os.File
		var err error
		if isWrite {
			f, err = os.OpenFile(stagingPath, os.O_RDWR, 0644)
		} else {
			f, err = os.OpenFile(stagingPath, os.O_RDONLY, 0644)
		}
		if err != nil {
			log.Printf("FUSE Open: error opening staging file: %v", err)
			return -fuse.EIO, 0
		}

		// Track as pending file if this is a write (overwriting)
		if isWrite {
			fs.pendingFilesMu.Lock()
			fs.pendingFiles[path] = &PendingFile{
				Path:        path,
				StagingPath: stagingPath,
				Size:        0,
				CreatedAt:   time.Now(),
			}
			fs.pendingFilesMu.Unlock()
		}

		handle := &FileHandle{
			Path:     path,
			File:     f,
			ReadOnly: !isWrite,
		}
		fh := fs.allocHandle(handle)
		return 0, fh
	}

	// If this is a write operation and file isn't in staging, create it
	if isWrite {
		if err := os.MkdirAll(filepath.Dir(stagingPath), 0755); err != nil {
			log.Printf("Open error creating staging dir: %v", err)
			return -fuse.EIO, 0
		}

		f, err := os.OpenFile(stagingPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			log.Printf("Open error creating staging file: %v", err)
			return -fuse.EIO, 0
		}

		// Track as pending file
		fs.pendingFilesMu.Lock()
		fs.pendingFiles[path] = &PendingFile{
			Path:        path,
			StagingPath: stagingPath,
			Size:        0,
			CreatedAt:   time.Now(),
		}
		fs.pendingFilesMu.Unlock()

		handle := &FileHandle{
			Path:     path,
			File:     f,
			ReadOnly: false,
		}
		fh := fs.allocHandle(handle)
		return 0, fh
	}

	// Check database for remote files (read-only access)
	var file *database.File
	var err error

	// If we have a provider name, look up in that provider's namespace
	if pathInfo.ProviderName != "" {
		providerID, pErr := fs.getProviderID(pathInfo.ProviderName)
		if pErr != nil {
			return -fuse.EIO, 0
		}
		if providerID == 0 {
			return -fuse.ENOENT, 0
		}
		// Use VirtualPath which includes the provider prefix (e.g., "/OneDrive/Documents/file.txt")
		file, err = fs.db.GetFileByProviderPath(fs.ctx, providerID, pathInfo.VirtualPath)
	} else {
		// Fallback for legacy paths
		file, err = fs.db.GetFileByPath(fs.ctx, path)
	}

	if err != nil {
		return -fuse.EIO, 0
	}

	if file == nil {
		return -fuse.ENOENT, 0
	}

	// Check if file is cached
	cache, err := fs.db.GetCacheEntry(fs.ctx, file.ID)
	if err != nil {
		return -fuse.EIO, 0
	}

	var handle *FileHandle
	var localPath string

	if cache != nil {
		// File is cached, verify it still exists
		if _, err := os.Stat(cache.LocalPath); err == nil {
			localPath = cache.LocalPath
			fs.db.TouchCacheEntry(fs.ctx, cache.ID)
		} else {
			// Cache entry is stale, remove it
			fs.db.DeleteCacheEntry(fs.ctx, cache.ID)
			cache = nil
		}
	}

	if cache == nil {
		// File not cached - defer download until Read() is called
		handle = &FileHandle{
			Path:      path,
			File:      nil, // Will be opened in Read()
			ReadOnly:  true,
			FileEntry: file,
		}

		fh := fs.allocHandle(handle)
		return 0, fh
	}

	// Open the local file (either from cache or freshly downloaded)
	f, err := os.Open(localPath)
	if err != nil {
		log.Printf("FUSE Open: failed to open cached file %s: %v", localPath, err)
		return -fuse.EIO, 0
	}

	handle = &FileHandle{
		Path:      path,
		File:      f,
		ReadOnly:  true,
		FileEntry: file,
	}

	fh := fs.allocHandle(handle)
	return 0, fh
}

// Read reads data from a file
func (fs *CloudUnifyFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
	handle := fs.getHandle(fh)
	if handle == nil {
		return -fuse.EBADF
	}

	// Handle deferred download
	if handle.File == nil {
		if handle.FileEntry == nil {
			return -fuse.EIO
		}

		// Deferred download - execute now
		// Notify progress: starting
		if fs.progressCallback != nil {
			fs.progressCallback(path, 0, handle.FileEntry.SizeBytes, "starting")
		}

		localPath, err := fs.downloadToCache(handle.FileEntry)
		if err != nil {
			log.Printf("FUSE Read: download failed for %s: %v", path, err)
			// Notify progress: error
			if fs.progressCallback != nil {
				fs.progressCallback(path, 0, handle.FileEntry.SizeBytes, "error")
			}
			return -fuse.EIO
		}

		f, err := os.Open(localPath)
		if err != nil {
			log.Printf("FUSE Read: failed to open cached file %s: %v", localPath, err)
			return -fuse.EIO
		}

		handle.File = f
	}

	n, err := handle.File.ReadAt(buff, ofst)
	if err != nil && n == 0 {
		return -fuse.EIO
	}

	return n
}

// Write writes data to a file
func (fs *CloudUnifyFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
	handle := fs.getHandle(fh)
	if handle == nil {
		log.Printf("FUSE Write: invalid handle for %s", path)
		return -fuse.EBADF
	}

	if handle.File == nil {
		log.Printf("FUSE Write: nil file for %s", path)
		return -fuse.EIO
	}

	n, err := handle.File.WriteAt(buff, ofst)
	if err != nil {
		log.Printf("Write error for %s at offset %d: %v", path, ofst, err)
		return -fuse.EIO
	}

	return n
}

// Release closes a file handle
func (fs *CloudUnifyFS) Release(path string, fh uint64) int {
	handle := fs.getHandle(fh)
	if handle == nil {
		return 0
	}

	if handle.File != nil {
		handle.File.Close()

		// If this was a write, queue for upload (only once)
		if !handle.ReadOnly {
			fs.pendingFilesMu.Lock()
			pending, exists := fs.pendingFiles[path]
			// Only queue upload if this file hasn't been queued yet
			if exists && !pending.Queued {
				pending.Queued = true
				stagingPath := fs.getStagingPath(path)
				// Get actual file size
				if info, err := os.Stat(stagingPath); err == nil {
					pending.Size = info.Size()
				}
				fs.syncEngine.EnqueueUpload(fs.ctx, path, stagingPath, 0)
			}
			fs.pendingFilesMu.Unlock()
		}
	}

	fs.freeHandle(fh)
	return 0
}

// Unlink deletes a file
func (fs *CloudUnifyFS) Unlink(path string) int {
	// First check if it's a pending file (not yet uploaded)
	fs.pendingFilesMu.Lock()
	if _, isPending := fs.pendingFiles[path]; isPending {
		delete(fs.pendingFiles, path)
		fs.pendingFilesMu.Unlock()

		// Delete staging file
		stagingPath := fs.getStagingPath(path)
		os.Remove(stagingPath)
		return 0
	}
	fs.pendingFilesMu.Unlock()

	// Check if file exists in database
	file, err := fs.db.GetFileByPath(fs.ctx, path)
	if err != nil {
		log.Printf("Unlink error getting file: %v", err)
		return -fuse.EIO
	}

	if file == nil {
		// Maybe it's a staging file that was never tracked
		stagingPath := fs.getStagingPath(path)
		if _, err := os.Stat(stagingPath); err == nil {
			os.Remove(stagingPath)
			return 0
		}
		return -fuse.ENOENT
	}

	// Capture file info before deleting from database
	cloudFileID := file.CloudFileID
	providerID := file.ProviderID

	// Delete from database immediately so it disappears from Finder
	if err := fs.db.DeleteFile(fs.ctx, file.ID); err != nil {
		log.Printf("Unlink error deleting from db: %v", err)
		return -fuse.EIO
	}

	// Queue cloud delete operation (async) with file info
	fs.syncEngine.EnqueueDelete(fs.ctx, path, cloudFileID, providerID, 0)

	// Also clean up any local staging/cache
	stagingPath := fs.getStagingPath(path)
	os.Remove(stagingPath)

	return 0
}

// Rename renames a file or directory
func (fs *CloudUnifyFS) Rename(oldpath string, newpath string) int {
	file, err := fs.db.GetFileByPath(fs.ctx, oldpath)
	if err != nil {
		return -fuse.EIO
	}

	if file == nil {
		return -fuse.ENOENT
	}

	// Update virtual path in database
	file.VirtualPath = newpath
	if err := fs.db.UpdateFile(fs.ctx, file); err != nil {
		return -fuse.EIO
	}

	return 0
}

// Truncate changes file size
func (fs *CloudUnifyFS) Truncate(path string, size int64, fh uint64) int {
	if fh != 0 {
		handle := fs.getHandle(fh)
		if handle != nil && handle.File != nil {
			if err := handle.File.Truncate(size); err != nil {
				log.Printf("Truncate error: %v", err)
				return -fuse.EIO
			}
			return 0
		}
	}

	// Also handle truncate without file handle (for staging files)
	stagingPath := fs.getStagingPath(path)
	if f, err := os.OpenFile(stagingPath, os.O_WRONLY, 0644); err == nil {
		defer f.Close()
		if err := f.Truncate(size); err != nil {
			log.Printf("Truncate staging error: %v", err)
			return -fuse.EIO
		}
		return 0
	}

	return 0
}

// Flush flushes cached data
func (fs *CloudUnifyFS) Flush(path string, fh uint64) int {
	handle := fs.getHandle(fh)
	if handle == nil {
		return 0
	}

	if handle.File != nil {
		if err := handle.File.Sync(); err != nil {
			log.Printf("Flush sync error: %v", err)
			return -fuse.EIO
		}
	}

	return 0
}

// Setattr sets file attributes
func (fs *CloudUnifyFS) Setattr(path string, stat *fuse.Stat_t, fh uint64) int {
	// Accept setattr for pending files and files in staging
	fs.pendingFilesMu.RLock()
	_, isPending := fs.pendingFiles[path]
	fs.pendingFilesMu.RUnlock()

	if isPending {
		return 0
	}

	// Check staging
	stagingPath := fs.getStagingPath(path)
	if _, err := os.Stat(stagingPath); err == nil {
		return 0
	}

	// Check database
	file, err := fs.db.GetFileByPath(fs.ctx, path)
	if err != nil {
		return -fuse.EIO
	}
	if file == nil {
		return -fuse.ENOENT
	}

	return 0
}

// Utimens sets file times
func (fs *CloudUnifyFS) Utimens(path string, tmsp []fuse.Timespec) int {
	return 0 // Accept but don't do anything
}

// Chmod changes file permissions
func (fs *CloudUnifyFS) Chmod(path string, mode uint32) int {
	return 0 // Accept but don't do anything
}

// Chown changes file ownership
func (fs *CloudUnifyFS) Chown(path string, uid uint32, gid uint32) int {
	return 0 // Accept but don't do anything
}

// Extended attribute support - needed for macOS Finder compatibility

// Getxattr gets an extended attribute
func (fs *CloudUnifyFS) Getxattr(path string, name string) (int, []byte) {
	// We don't store extended attributes, return ENOATTR (or ENODATA on Linux)
	return -fuse.ENOATTR, nil
}

// Setxattr sets an extended attribute
func (fs *CloudUnifyFS) Setxattr(path string, name string, value []byte, flags int) int {
	// Accept but don't store - silently succeed
	return 0
}

// Listxattr lists extended attributes
func (fs *CloudUnifyFS) Listxattr(path string, fill func(name string) bool) int {
	// No extended attributes to list
	return 0
}

// Removexattr removes an extended attribute
func (fs *CloudUnifyFS) Removexattr(path string, name string) int {
	// Accept removal even though we don't store them
	return 0
}

// Helper functions

func (fs *CloudUnifyFS) getStagingPath(virtualPath string) string {
	// Convert virtual path to staging path
	cleaned := strings.TrimPrefix(virtualPath, "/")
	return filepath.Join(fs.stagingDir, cleaned)
}

func (fs *CloudUnifyFS) allocHandle(handle *FileHandle) uint64 {
	fs.handlesMu.Lock()
	defer fs.handlesMu.Unlock()

	fh := fs.nextFH
	fs.nextFH++
	fs.handles[fh] = handle
	return fh
}

func (fs *CloudUnifyFS) getHandle(fh uint64) *FileHandle {
	fs.handlesMu.RLock()
	defer fs.handlesMu.RUnlock()
	return fs.handles[fh]
}

func (fs *CloudUnifyFS) freeHandle(fh uint64) {
	fs.handlesMu.Lock()
	defer fs.handlesMu.Unlock()
	delete(fs.handles, fh)
}

// Mount mounts the filesystem
func (fs *CloudUnifyFS) Mount() error {
	// Ensure mount point exists
	if err := os.MkdirAll(fs.mountPath, 0755); err != nil {
		return err
	}

	host := fuse.NewFileSystemHost(fs)

	// Platform-specific mount options
	options := []string{
		"-o", "allow_other",
	}

	go func() {
		if !host.Mount(fs.mountPath, options) {
			log.Printf("Failed to mount filesystem at %s", fs.mountPath)
		}
	}()

	return nil
}
