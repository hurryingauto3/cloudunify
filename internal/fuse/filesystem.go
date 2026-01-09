package fuse

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"

	"cloudunify/internal/database"
	"cloudunify/internal/sync"
)

// CloudUnifyFS implements the FUSE filesystem interface
type CloudUnifyFS struct {
	fuse.FileSystemBase

	db         *database.DB
	syncEngine *sync.Engine
	mountPath  string
	stagingDir string

	// File handles
	handles   map[uint64]*FileHandle
	handlesMu gosync.RWMutex
	nextFH    uint64

	// Pending files (files being written but not yet uploaded)
	pendingFiles   map[string]*PendingFile
	pendingFilesMu gosync.RWMutex

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
func NewCloudUnifyFS(db *database.DB, syncEngine *sync.Engine, mountPath, stagingDir string) *CloudUnifyFS {
	ctx, cancel := context.WithCancel(context.Background())
	return &CloudUnifyFS{
		db:           db,
		syncEngine:   syncEngine,
		mountPath:    mountPath,
		stagingDir:   stagingDir,
		handles:      make(map[uint64]*FileHandle),
		pendingFiles: make(map[string]*PendingFile),
		nextFH:       1,
		ctx:          ctx,
		cancel:       cancel,
	}
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
	// Root directory
	if path == "/" {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
		stat.Uid = uint32(os.Getuid())
		stat.Gid = uint32(os.Getgid())
		now := time.Now()
		stat.Atim = fuse.NewTimespec(now)
		stat.Mtim = fuse.NewTimespec(now)
		stat.Ctim = fuse.NewTimespec(now)
		return 0
	}

	// Check for pending files first (files being written)
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

	// Look up file in database
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

	// Track names we've seen
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
	log.Printf("FUSE Create: %s", path)
	
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

// Open opens a file
func (fs *CloudUnifyFS) Open(path string, flags int) (int, uint64) {
	log.Printf("FUSE Open: %s, flags=%d", path, flags)
	
	// Check if this is a write operation
	isWrite := (flags & (fuse.O_WRONLY | fuse.O_RDWR | fuse.O_CREAT | fuse.O_TRUNC)) != 0
	
	// First check if file is in staging directory (recently uploaded or pending)
	stagingPath := fs.getStagingPath(path)
	if _, err := os.Stat(stagingPath); err == nil {
		log.Printf("FUSE Open: found in staging: %s, isWrite=%v", stagingPath, isWrite)
		
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
		log.Printf("FUSE Open: creating staging file for write: %s", path)
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
	file, err := fs.db.GetFileByPath(fs.ctx, path)
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
	if cache != nil {
		// File is cached, open it
		f, err := os.Open(cache.LocalPath)
		if err != nil {
			return -fuse.EIO, 0
		}
		handle = &FileHandle{
			Path:      path,
			File:      f,
			ReadOnly:  true,
			FileEntry: file,
		}
		fs.db.TouchCacheEntry(fs.ctx, cache.ID)
	} else {
		// File not cached, need to download
		// For now, return a handle that will trigger download on read
		handle = &FileHandle{
			Path:      path,
			File:      nil, // Will be populated when download completes
			ReadOnly:  true,
			FileEntry: file,
		}
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

	if handle.File == nil {
		// File not yet downloaded - return 0 for now
		// In a full implementation, this would block and download
		log.Printf("Read: file not cached, would download: %s", path)
		return 0
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
				log.Printf("FUSE queuing upload: %s -> %s (size: %d)", path, stagingPath, pending.Size)
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
	file, err := fs.db.GetFileByPath(fs.ctx, path)
	if err != nil {
		return -fuse.EIO
	}

	if file == nil {
		return -fuse.ENOENT
	}

	// Queue delete operation
	fs.syncEngine.EnqueueDelete(fs.ctx, path, 0)

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
	log.Printf("FUSE Truncate: %s, size=%d, fh=%d", path, size, fh)
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
	log.Printf("FUSE Flush: %s, fh=%d", path, fh)
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
