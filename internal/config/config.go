package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Config holds the application configuration
type Config struct {
	MountPath string       `json:"mount_path"`
	Cache     CacheConfig  `json:"cache"`
	Sync      SyncConfig   `json:"sync"`
	API       APIConfig    `json:"api"`
	Logging   LogConfig    `json:"logging"`
	OAuth     OAuthConfigs `json:"oauth"`

	// AllocationStrategy determines how files are distributed across providers
	// Options: "balanced_usage", "most_free_space", "round_robin"
	AllocationStrategy string `json:"allocation_strategy"`
}

// OAuthConfigs holds OAuth credentials for all providers
type OAuthConfigs struct {
	GoogleDrive OAuthCredentials `json:"google_drive"`
	OneDrive    OAuthCredentials `json:"onedrive"`
	ICloud      ICloudConfig     `json:"icloud"`
}

// OAuthCredentials holds OAuth2 client credentials
type OAuthCredentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
}

// ICloudConfig holds iCloud-specific configuration (uses app-specific password)
type ICloudConfig struct {
	Username string `json:"username"`
	Password string `json:"password"` // App-specific password
}

// CacheConfig holds cache-related settings
type CacheConfig struct {
	Enabled   bool   `json:"enabled"`
	MaxSizeGB int    `json:"max_size_gb"`
	Path      string `json:"path"`
}

// RetryPolicy holds retry settings per error type
type RetryPolicy struct {
	NetworkRetries int `json:"network_retries"` // Retries for network errors (default: 5)
	AuthRetries    int `json:"auth_retries"`    // Retries for auth errors (default: 1)
	QuotaRetries   int `json:"quota_retries"`   // Retries for quota errors (default: 0)
	RateLimitRetries int `json:"rate_limit_retries"` // Retries for rate limiting (default: 5)
}

// SyncConfig holds sync engine settings
type SyncConfig struct {
	UploadWorkers   int  `json:"upload_workers"`
	DownloadWorkers int  `json:"download_workers"`
	AutoSync        bool `json:"auto_sync"`
	
	// Advanced settings (live reload)
	DownloadTimeoutSeconds    int         `json:"download_timeout_seconds"`     // Timeout for blocking downloads (default: 30, range: 5-300)
	RetryPolicy               RetryPolicy `json:"retry_policy"`                 // Retry counts per error type
	CompletedJobRetentionHours int        `json:"completed_job_retention_hours"` // Hours to keep completed jobs (default: 24, range: 1-168)
	StaleJobTimeoutMinutes    int         `json:"stale_job_timeout_minutes"`    // Minutes before processing jobs are considered stale (default: 30, range: 5-120)
}

// APIConfig holds HTTP API server settings
type APIConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// LogConfig holds logging settings
type LogConfig struct {
	Level string `json:"level"` // debug, info, warn, error
	File  string `json:"file"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig(paths *Paths) *Config {
	return &Config{
		MountPath: paths.MountPoint,
		Cache: CacheConfig{
			Enabled:   true,
			MaxSizeGB: 10,
			Path:      paths.CacheDir,
		},
		Sync: SyncConfig{
			UploadWorkers:   3,
			DownloadWorkers: 5,
			AutoSync:        true,
			DownloadTimeoutSeconds:    30,
			RetryPolicy: RetryPolicy{
				NetworkRetries:   5,
				AuthRetries:      1,
				QuotaRetries:     0,
				RateLimitRetries: 5,
			},
			CompletedJobRetentionHours: 24,
			StaleJobTimeoutMinutes:     30,
		},
		API: APIConfig{
			Host: "localhost",
			Port: 8080,
		},
		Logging: LogConfig{
			Level: "info",
			File:  paths.LogFilePath(),
		},
		OAuth: OAuthConfigs{
			GoogleDrive: OAuthCredentials{
				ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
				ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
				RedirectURL:  "http://localhost:8080/api/auth/callback",
			},
			OneDrive: OAuthCredentials{
				ClientID:     os.Getenv("ONEDRIVE_CLIENT_ID"),
				ClientSecret: os.Getenv("ONEDRIVE_CLIENT_SECRET"),
				RedirectURL:  "http://localhost:8080/api/auth/callback",
			},
			ICloud: ICloudConfig{
				Username: os.Getenv("ICLOUD_USERNAME"),
				Password: os.Getenv("ICLOUD_PASSWORD"),
			},
		},
		AllocationStrategy: "balanced_usage",
	}
}

// Manager handles loading and saving configuration
type Manager struct {
	config *Config
	paths  *Paths
	mu     sync.RWMutex
}

// NewManager creates a new configuration manager
func NewManager() (*Manager, error) {
	paths, err := GetPaths()
	if err != nil {
		return nil, fmt.Errorf("failed to get paths: %w", err)
	}

	if err := paths.EnsureDirectories(); err != nil {
		return nil, fmt.Errorf("failed to create directories: %w", err)
	}

	m := &Manager{
		paths:  paths,
		config: DefaultConfig(paths),
	}

	// Try to load existing config, use defaults if not found
	if err := m.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return m, nil
}

// Load reads configuration from the config file
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	configPath := m.paths.ConfigFilePath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	m.config = &config
	return nil
}

// Save writes the current configuration to the config file
func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configPath := m.paths.ConfigFilePath()
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// Get returns a copy of the current configuration
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return *m.config
}

// Update applies changes to the configuration
func (m *Manager) Update(fn func(*Config)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fn(m.config)
	return nil
}

// Paths returns the paths configuration
func (m *Manager) Paths() *Paths {
	return m.paths
}

// APIAddress returns the full API server address
func (c *Config) APIAddress() string {
	return fmt.Sprintf("%s:%d", c.API.Host, c.API.Port)
}

// ValidateSyncConfig validates sync configuration values and clamps them to valid ranges
func ValidateSyncConfig(sync *SyncConfig) {
	// Download timeout: 5-300 seconds
	if sync.DownloadTimeoutSeconds < 5 {
		sync.DownloadTimeoutSeconds = 5
	} else if sync.DownloadTimeoutSeconds > 300 {
		sync.DownloadTimeoutSeconds = 300
	}

	// Retry counts: 0-10
	if sync.RetryPolicy.NetworkRetries < 0 {
		sync.RetryPolicy.NetworkRetries = 0
	} else if sync.RetryPolicy.NetworkRetries > 10 {
		sync.RetryPolicy.NetworkRetries = 10
	}

	if sync.RetryPolicy.AuthRetries < 0 {
		sync.RetryPolicy.AuthRetries = 0
	} else if sync.RetryPolicy.AuthRetries > 10 {
		sync.RetryPolicy.AuthRetries = 10
	}

	if sync.RetryPolicy.QuotaRetries < 0 {
		sync.RetryPolicy.QuotaRetries = 0
	} else if sync.RetryPolicy.QuotaRetries > 10 {
		sync.RetryPolicy.QuotaRetries = 10
	}

	if sync.RetryPolicy.RateLimitRetries < 0 {
		sync.RetryPolicy.RateLimitRetries = 0
	} else if sync.RetryPolicy.RateLimitRetries > 10 {
		sync.RetryPolicy.RateLimitRetries = 10
	}

	// Job retention: 1-168 hours (1 week)
	if sync.CompletedJobRetentionHours < 1 {
		sync.CompletedJobRetentionHours = 1
	} else if sync.CompletedJobRetentionHours > 168 {
		sync.CompletedJobRetentionHours = 168
	}

	// Stale job timeout: 5-120 minutes
	if sync.StaleJobTimeoutMinutes < 5 {
		sync.StaleJobTimeoutMinutes = 5
	} else if sync.StaleJobTimeoutMinutes > 120 {
		sync.StaleJobTimeoutMinutes = 120
	}

	// Worker counts: 1-10
	if sync.UploadWorkers < 1 {
		sync.UploadWorkers = 1
	} else if sync.UploadWorkers > 10 {
		sync.UploadWorkers = 10
	}

	if sync.DownloadWorkers < 1 {
		sync.DownloadWorkers = 1
	} else if sync.DownloadWorkers > 10 {
		sync.DownloadWorkers = 10
	}
}

// GetSyncConfig returns a copy of the sync configuration (thread-safe)
func (m *Manager) GetSyncConfig() SyncConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.Sync
}

// UpdateSyncConfig updates the sync configuration (live reload for most settings)
// Returns true if a restart is required (worker count changed)
func (m *Manager) UpdateSyncConfig(newSync SyncConfig) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worker counts changed (requires restart)
	restartRequired := m.config.Sync.UploadWorkers != newSync.UploadWorkers ||
		m.config.Sync.DownloadWorkers != newSync.DownloadWorkers

	// Validate and apply
	ValidateSyncConfig(&newSync)
	m.config.Sync = newSync

	return restartRequired
}
