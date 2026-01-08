package database

import (
	"database/sql"
	"time"
)

// ProviderType represents the type of cloud storage provider
type ProviderType string

const (
	ProviderGoogleDrive ProviderType = "google_drive"
	ProviderOneDrive    ProviderType = "onedrive"
	ProviderICloud      ProviderType = "icloud"
)

// FileStatus represents the sync status of a file
type FileStatus string

const (
	FileStatusSynced    FileStatus = "synced"
	FileStatusUploading FileStatus = "uploading"
	FileStatusPending   FileStatus = "pending"
	FileStatusError     FileStatus = "error"
)

// SyncOperation represents the type of sync operation
type SyncOperation string

const (
	SyncOpUpload   SyncOperation = "upload"
	SyncOpDownload SyncOperation = "download"
	SyncOpDelete   SyncOperation = "delete"
)

// SyncStatus represents the status of a sync queue item
type SyncStatus string

const (
	SyncStatusPending    SyncStatus = "pending"
	SyncStatusProcessing SyncStatus = "processing"
	SyncStatusCompleted  SyncStatus = "completed"
	SyncStatusFailed     SyncStatus = "failed"
)

// Provider represents a cloud storage provider configuration
type Provider struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	Type         ProviderType `json:"type"`
	Enabled      bool         `json:"enabled"`
	QuotaBytes   int64        `json:"quota_bytes"`
	UsedBytes    int64        `json:"used_bytes"`
	AccessToken  string       `json:"-"` // Never expose in JSON
	RefreshToken string       `json:"-"` // Never expose in JSON
	TokenExpiry  *time.Time   `json:"token_expiry,omitempty"`
	Config       string       `json:"config,omitempty"` // JSON string for provider-specific config
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// FreeBytes returns the available storage space for this provider
func (p *Provider) FreeBytes() int64 {
	return p.QuotaBytes - p.UsedBytes
}

// UsagePercent returns the percentage of storage used (0-100)
func (p *Provider) UsagePercent() float64 {
	if p.QuotaBytes == 0 {
		return 0
	}
	return float64(p.UsedBytes) / float64(p.QuotaBytes) * 100
}

// File represents a file in the virtual filesystem
type File struct {
	ID          int64      `json:"id"`
	VirtualPath string     `json:"virtual_path"`
	ProviderID  int64      `json:"provider_id"`
	CloudFileID string     `json:"cloud_file_id"`
	CloudPath   string     `json:"cloud_path,omitempty"`
	SizeBytes   int64      `json:"size_bytes"`
	Checksum    string     `json:"checksum,omitempty"`
	MimeType    string     `json:"mime_type,omitempty"`
	Status      FileStatus `json:"status"`
	IsDir       bool       `json:"is_dir"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SyncQueueItem represents an item in the sync queue
type SyncQueueItem struct {
	ID              int64         `json:"id"`
	Operation       SyncOperation `json:"operation"`
	VirtualPath     string        `json:"virtual_path"`
	LocalPath       string        `json:"local_path,omitempty"`
	ProviderID      sql.NullInt64 `json:"provider_id,omitempty"`
	Priority        int           `json:"priority"`
	Status          SyncStatus    `json:"status"`
	ProgressPercent int           `json:"progress_percent"`
	ErrorMessage    string        `json:"error_message,omitempty"`
	RetryCount      int           `json:"retry_count"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

// CacheEntry represents a cached file
type CacheEntry struct {
	ID           int64     `json:"id"`
	FileID       int64     `json:"file_id"`
	LocalPath    string    `json:"local_path"`
	SizeBytes    int64     `json:"size_bytes"`
	LastAccessed time.Time `json:"last_accessed"`
}

// ProviderSummary provides a summary of a provider's storage
type ProviderSummary struct {
	Provider  *Provider `json:"provider"`
	FileCount int       `json:"file_count"`
}

// StorageSummary provides aggregated storage statistics
type StorageSummary struct {
	TotalBytes int64             `json:"total_bytes"`
	UsedBytes  int64             `json:"used_bytes"`
	FreeBytes  int64             `json:"free_bytes"`
	Providers  []ProviderSummary `json:"providers"`
}
