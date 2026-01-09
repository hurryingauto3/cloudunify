package providers

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
)

// Common errors
var (
	ErrNotAuthenticated = errors.New("provider not authenticated")
	ErrTokenExpired     = errors.New("token expired")
	ErrQuotaExceeded    = errors.New("storage quota exceeded")
	ErrFileNotFound     = errors.New("file not found")
	ErrRateLimited      = errors.New("rate limited by provider")
	ErrNetworkError     = errors.New("network error")
)

// FileMetadata represents metadata about a file in cloud storage
type FileMetadata struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	MimeType    string    `json:"mime_type"`
	Checksum    string    `json:"checksum"`
	ModTime     time.Time `json:"mod_time"`
	IsDir       bool      `json:"is_dir"`
	DownloadURL string    `json:"download_url,omitempty"`
}

// QuotaInfo represents storage quota information
type QuotaInfo struct {
	TotalBytes int64 `json:"total_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
	FreeBytes  int64 `json:"free_bytes"`
}

// AuthConfig holds OAuth configuration for a provider
type AuthConfig struct {
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	RedirectURL  string   `json:"redirect_url"`
	Scopes       []string `json:"scopes"`
}

// TokenInfo holds OAuth token information
type TokenInfo struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

// CloudProvider defines the interface that all cloud storage providers must implement
type CloudProvider interface {
	// Type returns the provider type identifier
	Type() string

	// Name returns the display name for this provider instance
	Name() string

	// Authentication
	// GetAuthURL returns the OAuth authorization URL for user authentication
	GetAuthURL(state string) string

	// ExchangeCode exchanges an authorization code for tokens
	ExchangeCode(ctx context.Context, code string) (*TokenInfo, error)

	// SetTokens sets the OAuth tokens for this provider
	SetTokens(tokens *TokenInfo)

	// RefreshToken refreshes the access token using the refresh token
	RefreshToken(ctx context.Context) (*TokenInfo, error)

	// IsAuthenticated returns whether the provider has valid tokens
	IsAuthenticated() bool

	// File Operations
	// Upload uploads a file from a local path to the cloud
	Upload(ctx context.Context, localPath string, remotePath string) (*FileMetadata, error)

	// UploadStream uploads data from a reader to the cloud
	UploadStream(ctx context.Context, reader io.Reader, remotePath string, size int64) (*FileMetadata, error)

	// Download downloads a file to a writer
	Download(ctx context.Context, fileID string, writer io.Writer) error

	// DownloadStream returns a reader for streaming download
	DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, error)

	// Delete removes a file from the cloud
	Delete(ctx context.Context, fileID string) error

	// Metadata Operations
	// GetFile retrieves metadata for a specific file
	GetFile(ctx context.Context, fileID string) (*FileMetadata, error)

	// ListFiles lists files in a directory
	ListFiles(ctx context.Context, path string) ([]*FileMetadata, error)

	// CreateFolder creates a new folder
	CreateFolder(ctx context.Context, path string) (*FileMetadata, error)

	// Storage Information
	// GetQuota returns the storage quota information
	GetQuota(ctx context.Context) (*QuotaInfo, error)
}

// ProviderFactory creates provider instances
type ProviderFactory struct {
	configs map[string]*AuthConfig
}

// NewProviderFactory creates a new provider factory
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{
		configs: make(map[string]*AuthConfig),
	}
}

// RegisterConfig registers an OAuth configuration for a provider type
func (f *ProviderFactory) RegisterConfig(providerType string, config *AuthConfig) {
	f.configs[providerType] = config
}

// CreateProvider creates a new provider instance
func (f *ProviderFactory) CreateProvider(providerType string, name string) (CloudProvider, error) {
	config, ok := f.configs[providerType]
	if !ok {
		return nil, errors.New("provider type not registered: " + providerType)
	}

	switch providerType {
	case "google_drive":
		return NewGoogleDriveProvider(name, config), nil
	case "onedrive":
		return NewOneDriveProvider(name, config), nil
	case "icloud":
		return NewICloudProvider(name, config), nil
	default:
		return nil, errors.New("unknown provider type: " + providerType)
	}
}

// ProgressCallback is called during uploads/downloads to report progress
type ProgressCallback func(bytesTransferred int64, totalBytes int64)

// ProgressReader wraps an io.Reader to track progress
type ProgressReader struct {
	reader           io.Reader
	total            int64
	transferred      int64
	progressCallback ProgressCallback
}

// NewProgressReader creates a new progress-tracking reader
func NewProgressReader(reader io.Reader, total int64, callback ProgressCallback) *ProgressReader {
	return &ProgressReader{
		reader:           reader,
		total:            total,
		progressCallback: callback,
	}
}

// Read implements io.Reader
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.transferred += int64(n)
	if pr.progressCallback != nil {
		pr.progressCallback(pr.transferred, pr.total)
	}
	return n, err
}

// IsRetriableError returns whether an error should trigger a retry
func IsRetriableError(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are retriable
	if errors.Is(err, ErrNetworkError) {
		return true
	}

	// Rate limiting is retriable (after backoff)
	if errors.Is(err, ErrRateLimited) {
		return true
	}

	// Token expired might be retriable after refresh
	if errors.Is(err, ErrTokenExpired) {
		return true
	}

	// These are not retriable
	if errors.Is(err, ErrNotAuthenticated) ||
		errors.Is(err, ErrQuotaExceeded) ||
		errors.Is(err, ErrFileNotFound) {
		return false
	}

	// Unknown errors - be conservative and retry
	return true
}

// ErrorCategory represents the type of error for retry policy
type ErrorCategory string

const (
	ErrorCategoryNetwork   ErrorCategory = "network"
	ErrorCategoryAuth      ErrorCategory = "auth"
	ErrorCategoryQuota     ErrorCategory = "quota"
	ErrorCategoryRateLimit ErrorCategory = "rate_limit"
	ErrorCategoryNotFound  ErrorCategory = "not_found"
	ErrorCategoryUnknown   ErrorCategory = "unknown"
)

// ClassifyError returns the category of an error for retry policy decisions
func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return ErrorCategoryUnknown
	}

	if errors.Is(err, ErrNetworkError) {
		return ErrorCategoryNetwork
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrorCategoryRateLimit
	}
	if errors.Is(err, ErrTokenExpired) || errors.Is(err, ErrNotAuthenticated) {
		return ErrorCategoryAuth
	}
	if errors.Is(err, ErrQuotaExceeded) {
		return ErrorCategoryQuota
	}
	if errors.Is(err, ErrFileNotFound) {
		return ErrorCategoryNotFound
	}

	// Check error message for hints
	errMsg := err.Error()
	if contains(errMsg, "timeout", "connection", "network", "dial", "EOF", "reset") {
		return ErrorCategoryNetwork
	}
	if contains(errMsg, "401", "403", "unauthorized", "forbidden", "auth") {
		return ErrorCategoryAuth
	}
	if contains(errMsg, "429", "rate", "too many", "throttle") {
		return ErrorCategoryRateLimit
	}
	if contains(errMsg, "quota", "storage", "limit", "space") {
		return ErrorCategoryQuota
	}
	if contains(errMsg, "404", "not found") {
		return ErrorCategoryNotFound
	}

	return ErrorCategoryUnknown
}

// contains checks if s contains any of the substrings (case-insensitive)
func contains(s string, substrs ...string) bool {
	sl := strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(sl, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}
