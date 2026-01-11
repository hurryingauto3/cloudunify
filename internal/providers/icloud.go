package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// iCloudProvider implements CloudProvider for iCloud via local folder access
// On macOS/Windows, this reads/writes directly to the iCloud Drive folder,
// and the OS handles the sync to Apple's servers.
//
// Platform paths:
//   - macOS: ~/Library/Mobile Documents/com~apple~CloudDocs
//   - Windows: %USERPROFILE%\iCloudDrive (requires iCloud for Windows)
type ICloudProvider struct {
	name     string
	config   *AuthConfig
	tokens   *TokenInfo
	basePath string // Resolved path to iCloud folder
}

// NewICloudProvider creates a new iCloud provider
func NewICloudProvider(name string, config *AuthConfig) *ICloudProvider {
	return &ICloudProvider{
		name:   name,
		config: config,
	}
}

// Type returns the provider type identifier
func (p *ICloudProvider) Type() string {
	return "icloud"
}

// Name returns the display name for this provider instance
func (p *ICloudProvider) Name() string {
	return p.name
}

// getDefaultICloudPath returns the platform-specific iCloud Drive folder path
func getDefaultICloudPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "~/Library/Mobile Documents/com~apple~CloudDocs"
	case "windows":
		// iCloud for Windows default location
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, "iCloudDrive")
	default:
		// Linux - no official iCloud support
		return ""
	}
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// GetAuthURL returns instructions for iCloud setup
func (p *ICloudProvider) GetAuthURL(state string) string {
	// For local folder approach, we just need to verify the folder exists
	// Return a special URL that the frontend will handle
	return fmt.Sprintf("cloudunify://icloud-local?state=%s", state)
}

// ExchangeCode validates that iCloud Drive folder exists and is accessible
func (p *ICloudProvider) ExchangeCode(ctx context.Context, code string) (*TokenInfo, error) {
	// Get platform-specific default path
	basePath := getDefaultICloudPath()
	if basePath == "" {
		return nil, fmt.Errorf("iCloud Drive is not supported on %s. Only macOS and Windows (with iCloud for Windows) are supported", runtime.GOOS)
	}
	basePath = expandPath(basePath)

	// Check if custom path was provided
	if code != "" && code != "local" {
		basePath = expandPath(code)
	}

	// Verify the folder exists
	info, err := os.Stat(basePath)
	if err != nil {
		if os.IsNotExist(err) {
			if runtime.GOOS == "darwin" {
				return nil, fmt.Errorf("iCloud Drive folder not found at %s. Make sure iCloud Drive is enabled in System Preferences > Apple ID > iCloud", basePath)
			} else if runtime.GOOS == "windows" {
				return nil, fmt.Errorf("iCloud Drive folder not found at %s. Make sure iCloud for Windows is installed and iCloud Drive is enabled", basePath)
			}
			return nil, fmt.Errorf("iCloud Drive folder not found at %s", basePath)
		}
		return nil, fmt.Errorf("cannot access iCloud Drive folder: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("iCloud Drive path is not a directory: %s", basePath)
	}

	// Test write access
	testFile := filepath.Join(basePath, ".cloudunify_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return nil, fmt.Errorf("no write access to iCloud Drive folder: %w", err)
	}
	os.Remove(testFile)

	p.basePath = basePath

	// Return a token with the base path stored
	return &TokenInfo{
		AccessToken:  basePath, // Store the path as the "token"
		RefreshToken: "",
		TokenType:    "local",
		Expiry:       time.Now().Add(100 * 365 * 24 * time.Hour), // Never expires
	}, nil
}

// SetTokens sets the authentication credentials
func (p *ICloudProvider) SetTokens(tokens *TokenInfo) {
	p.tokens = tokens
	if tokens != nil && tokens.AccessToken != "" {
		p.basePath = tokens.AccessToken
	}
}

// RefreshToken for iCloud local is a no-op
func (p *ICloudProvider) RefreshToken(ctx context.Context) (*TokenInfo, error) {
	if p.tokens == nil {
		return nil, ErrNotAuthenticated
	}
	return p.tokens, nil
}

// IsAuthenticated returns whether the provider is configured
func (p *ICloudProvider) IsAuthenticated() bool {
	if p.tokens == nil || p.basePath == "" {
		return false
	}
	// Verify folder still exists
	_, err := os.Stat(p.basePath)
	return err == nil
}

// resolvePath converts a virtual path to the actual filesystem path
func (p *ICloudProvider) resolvePath(virtualPath string) string {
	virtualPath = strings.TrimPrefix(virtualPath, "/")
	// Treat "root" as empty (base path), also handle "root/..." paths
	if virtualPath == "root" {
		virtualPath = ""
	} else if strings.HasPrefix(virtualPath, "root/") {
		virtualPath = strings.TrimPrefix(virtualPath, "root/")
	}
	return filepath.Join(p.basePath, virtualPath)
}

// generateFileID creates a stable ID from the file path
func (p *ICloudProvider) generateFileID(path string) string {
	hash := sha256.Sum256([]byte(path))
	return "icloud_" + hex.EncodeToString(hash[:8])
}

// pathFromID extracts the path from our ID format (for local files, ID IS the path)
func (p *ICloudProvider) pathFromID(fileID string) string {
	// For local provider, the fileID is the relative path
	// We store the relative path directly as the ID in the database
	return fileID
}

// Upload uploads a file from a local path to iCloud
func (p *ICloudProvider) Upload(ctx context.Context, localPath string, remotePath string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	srcFile, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat source file: %w", err)
	}

	return p.UploadStream(ctx, srcFile, remotePath, srcInfo.Size())
}

// UploadStream uploads data from a reader to iCloud
func (p *ICloudProvider) UploadStream(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	destPath := p.resolvePath(remotePath)

	// Ensure parent directory exists
	parentDir := filepath.Dir(destPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create destination file
	destFile, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer destFile.Close()

	// Copy content
	written, err := io.Copy(destFile, reader)
	if err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Get file info for metadata
	info, err := os.Stat(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat new file: %w", err)
	}

	return &FileMetadata{
		ID:       strings.TrimPrefix(remotePath, "/"),
		Name:     filepath.Base(remotePath),
		Path:     remotePath,
		Size:     written,
		MimeType: detectMimeType(filepath.Base(remotePath)),
		ModTime:  info.ModTime(),
		IsDir:    false,
	}, nil
}

// Download downloads a file to a writer
func (p *ICloudProvider) Download(ctx context.Context, fileID string, writer io.Writer) error {
	if !p.IsAuthenticated() {
		return ErrNotAuthenticated
	}

	stream, err := p.DownloadStream(ctx, fileID)
	if err != nil {
		return err
	}
	defer stream.Close()

	_, err = io.Copy(writer, stream)
	return err
}

// DownloadStream returns a reader for streaming download
func (p *ICloudProvider) DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	filePath := p.resolvePath(fileID)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// SupportsRangeRequests returns true - local files support range reads
func (p *ICloudProvider) SupportsRangeRequests() bool {
	return true
}

// DownloadRange downloads a byte range from a file
func (p *ICloudProvider) DownloadRange(ctx context.Context, fileID string, start, end int64) (io.ReadCloser, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	filePath := p.resolvePath(fileID)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Seek to start position
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	// Return a limited reader that only reads the requested range
	length := end - start + 1
	return &limitedReadCloser{
		Reader: io.LimitReader(file, length),
		Closer: file,
	}, nil
}

// limitedReadCloser wraps a LimitReader with a Closer
type limitedReadCloser struct {
	io.Reader
	io.Closer
}

// Delete removes a file from iCloud
func (p *ICloudProvider) Delete(ctx context.Context, fileID string) error {
	if !p.IsAuthenticated() {
		return ErrNotAuthenticated
	}

	filePath := p.resolvePath(fileID)

	// Check if it exists
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return os.RemoveAll(filePath)
	}
	return os.Remove(filePath)
}

// GetFile retrieves metadata for a specific file
func (p *ICloudProvider) GetFile(ctx context.Context, fileID string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	filePath := p.resolvePath(fileID)

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &FileMetadata{
		ID:       fileID,
		Name:     info.Name(),
		Path:     fileID,
		Size:     info.Size(),
		MimeType: detectMimeType(info.Name()),
		ModTime:  info.ModTime(),
		IsDir:    info.IsDir(),
	}, nil
}

// ListFiles lists files in a directory
func (p *ICloudProvider) ListFiles(ctx context.Context, path string) ([]*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	dirPath := p.resolvePath(path)

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []*FileMetadata
	for _, entry := range entries {
		// Skip hidden files and system files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		relativePath := path
		// Treat "root" as base path for ID generation
		if relativePath == "" || relativePath == "/" || relativePath == "." || relativePath == "root" {
			relativePath = entry.Name()
		} else {
			// Strip "root/" prefix if present
			cleanPath := strings.TrimPrefix(path, "/")
			cleanPath = strings.TrimPrefix(cleanPath, "root/")
			relativePath = filepath.Join(cleanPath, entry.Name())
		}

		files = append(files, &FileMetadata{
			ID:       relativePath,
			Name:     entry.Name(),
			Path:     relativePath,
			Size:     info.Size(),
			MimeType: detectMimeType(entry.Name()),
			ModTime:  info.ModTime(),
			IsDir:    entry.IsDir(),
		})
	}

	return files, nil
}

// CreateFolder creates a new folder in iCloud
func (p *ICloudProvider) CreateFolder(ctx context.Context, path string) (*FileMetadata, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	folderPath := p.resolvePath(path)

	if err := os.MkdirAll(folderPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	info, err := os.Stat(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat folder: %w", err)
	}

	return &FileMetadata{
		ID:      strings.TrimPrefix(path, "/"),
		Name:    filepath.Base(path),
		Path:    path,
		IsDir:   true,
		ModTime: info.ModTime(),
	}, nil
}

// GetQuota returns the storage quota information
// For local folder approach, we check disk space and estimate iCloud usage
func (p *ICloudProvider) GetQuota(ctx context.Context) (*QuotaInfo, error) {
	if !p.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}

	// Calculate used space in iCloud folder
	var usedBytes int64
	err := filepath.Walk(p.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			usedBytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to calculate usage: %w", err)
	}

	// Try to get disk space (platform-specific)
	// Default to 5GB if we can't determine
	totalBytes := int64(5 * 1024 * 1024 * 1024) // 5GB default
	freeBytes := int64(5*1024*1024*1024) - usedBytes

	// On macOS/Unix, we could use syscall to get actual disk space
	// but for simplicity, we'll use the iCloud plan size from config
	// Users typically have 5GB (free), 50GB, 200GB, or 2TB plans
	if runtime.GOOS == "darwin" {
		// Try to estimate based on iCloud plan (default 5GB)
		// In a production app, you'd query the iCloud API for actual quota
		// For now, assume user's plan based on usage
		if usedBytes > 200*1024*1024*1024 {
			totalBytes = 2 * 1024 * 1024 * 1024 * 1024 // 2TB
		} else if usedBytes > 50*1024*1024*1024 {
			totalBytes = 200 * 1024 * 1024 * 1024 // 200GB
		} else if usedBytes > 5*1024*1024*1024 {
			totalBytes = 50 * 1024 * 1024 * 1024 // 50GB
		}
		freeBytes = totalBytes - usedBytes
	}

	return &QuotaInfo{
		TotalBytes: totalBytes,
		UsedBytes:  usedBytes,
		FreeBytes:  freeBytes,
	}, nil
}
