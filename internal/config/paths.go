package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths holds platform-specific directory paths
type Paths struct {
	ConfigDir  string // Configuration files
	DataDir    string // SQLite database, etc.
	CacheDir   string // File cache
	LogDir     string // Log files
	StagingDir string // Temporary upload staging
	MountPoint string // FUSE mount point
}

// GetPaths returns platform-specific paths for the application
func GetPaths() (*Paths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var paths Paths

	switch runtime.GOOS {
	case "darwin":
		// macOS paths follow Apple conventions
		paths.ConfigDir = filepath.Join(homeDir, "Library", "Application Support", "CloudUnify")
		paths.DataDir = filepath.Join(homeDir, "Library", "Application Support", "CloudUnify")
		paths.CacheDir = filepath.Join(homeDir, "Library", "Caches", "CloudUnify")
		paths.LogDir = filepath.Join(homeDir, "Library", "Logs", "CloudUnify")
		paths.StagingDir = filepath.Join(homeDir, "Library", "Caches", "CloudUnify", "staging")
		paths.MountPoint = filepath.Join(homeDir, "CloudUnify")

	case "windows":
		// Windows paths use APPDATA
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(homeDir, "AppData", "Local")
		}
		paths.ConfigDir = filepath.Join(appData, "CloudUnify")
		paths.DataDir = filepath.Join(appData, "CloudUnify")
		paths.CacheDir = filepath.Join(localAppData, "CloudUnify", "cache")
		paths.LogDir = filepath.Join(appData, "CloudUnify", "Logs")
		paths.StagingDir = filepath.Join(localAppData, "CloudUnify", "staging")
		paths.MountPoint = "C:\\CloudUnify"

	default:
		// Linux and other Unix-like systems follow XDG conventions
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(homeDir, ".config")
		}
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(homeDir, ".local", "share")
		}
		cacheHome := os.Getenv("XDG_CACHE_HOME")
		if cacheHome == "" {
			cacheHome = filepath.Join(homeDir, ".cache")
		}
		paths.ConfigDir = filepath.Join(configHome, "cloudunify")
		paths.DataDir = filepath.Join(dataHome, "cloudunify")
		paths.CacheDir = filepath.Join(cacheHome, "cloudunify")
		paths.LogDir = filepath.Join(dataHome, "cloudunify", "logs")
		paths.StagingDir = filepath.Join(cacheHome, "cloudunify", "staging")
		paths.MountPoint = filepath.Join(homeDir, "CloudUnify")
	}

	return &paths, nil
}

// EnsureDirectories creates all required directories if they don't exist
func (p *Paths) EnsureDirectories() error {
	dirs := []string{
		p.ConfigDir,
		p.DataDir,
		p.CacheDir,
		p.LogDir,
		p.StagingDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// ConfigFilePath returns the full path to the config file
func (p *Paths) ConfigFilePath() string {
	return filepath.Join(p.ConfigDir, "config.json")
}

// DatabasePath returns the full path to the SQLite database
func (p *Paths) DatabasePath() string {
	return filepath.Join(p.DataDir, "cloudunify.db")
}

// LogFilePath returns the full path to the main log file
func (p *Paths) LogFilePath() string {
	return filepath.Join(p.LogDir, "cloudunify.log")
}
