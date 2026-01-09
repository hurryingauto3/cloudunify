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

// SyncConfig holds sync engine settings
type SyncConfig struct {
	UploadWorkers   int  `json:"upload_workers"`
	DownloadWorkers int  `json:"download_workers"`
	AutoSync        bool `json:"auto_sync"`
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
