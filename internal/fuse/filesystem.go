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

	// Context for operations
	ctx    context.Context
	cancel context.CancelFunc
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
		db:         db,
		syncEngine: syncEngine,
		mountPath:  mountPath,
		stagingDir: stagingDir,
		handles:    make(map[uint64]*FileHandle),
		nextFH:     1,
		ctx:        ctx,
		cancel:     cancel,
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

	// Look up file in database
	file, err := fs.db.GetFileByPath(fs.ctx, path)
	if err != nil {
		return -fuse.EIO
	}

	if file == nil {
		return -fuse.ENOENT
	}

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

// Readdir reads directory contents
func (fs *CloudUnifyFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
	fill(".", nil, 0)
	fill("..", nil, 0)

	// List files in this directory
	files, err := fs.db.ListFilesInDirectory(fs.ctx, path)
	if err != nil {
		log.Printf("Readdir error: %v", err)
		return -fuse.EIO
	}

	for _, file := range files {
		name := filepath.Base(file.VirtualPath)
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
		return -fuse.EBADF
	}

	if handle.File == nil {
		return -fuse.EIO
	}

	n, err := handle.File.WriteAt(buff, ofst)
	if err != nil {
		log.Printf("Write error: %v", err)
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

		// If this was a write, queue for upload
		if !handle.ReadOnly {
			stagingPath := fs.getStagingPath(path)
			fs.syncEngine.EnqueueUpload(fs.ctx, path, stagingPath, 0)
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
	if fh != 0 {
		handle := fs.getHandle(fh)
		if handle != nil && handle.File != nil {
			if err := handle.File.Truncate(size); err != nil {
				return -fuse.EIO
			}
			return 0
		}
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
