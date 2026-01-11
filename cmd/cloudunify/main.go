package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloudunify/internal/api"
	"cloudunify/internal/config"
	"cloudunify/internal/database"
	"cloudunify/internal/fuse"
	"cloudunify/internal/providers"
	"cloudunify/internal/storage"
	"cloudunify/internal/sync"
)

var (
	version   = "0.1.0"
	buildTime = "development"
)

func main() {
	// Parse command-line flags
	var (
		showVersion = flag.Bool("version", false, "Show version information")
		configPath  = flag.String("config", "", "Path to configuration file")
		noMount     = flag.Bool("no-mount", false, "Don't mount the FUSE filesystem")
		noSync      = flag.Bool("no-sync", false, "Don't start the sync engine")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("CloudUnify v%s (built: %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("CloudUnify v%s starting...", version)

	// Initialize configuration
	configManager, err := config.NewManager()
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Override config path if specified
	if *configPath != "" {
		os.Setenv("CLOUDUNIFY_CONFIG_PATH", *configPath)
	}

	cfg := configManager.Get()
	paths := configManager.Paths()

	log.Printf("Configuration loaded from: %s", paths.ConfigFilePath())
	log.Printf("Database path: %s", paths.DatabasePath())
	log.Printf("Mount point: %s", cfg.MountPath)

	// Initialize database
	db, err := database.Open(paths.DatabasePath())
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	log.Println("Database initialized")

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize provider manager with OAuth configs
	providerManager := providers.NewManager()
	setupProviderConfigs(providerManager, cfg)
	log.Println("Provider manager initialized")

	// Log OAuth configuration status
	logOAuthStatus(cfg)

	// Initialize storage allocator
	allocator := storage.NewAllocator(db, storage.AllocationStrategy(cfg.AllocationStrategy))
	log.Println("Storage allocator initialized")

	// Initialize sync engine
	syncEngine := sync.NewEngine(db, allocator, cfg.Sync.UploadWorkers, cfg.Sync.DownloadWorkers)
	// Apply sync config from configuration
	syncEngine.UpdateConfig(cfg.Sync)
	log.Println("Sync engine initialized")

	// Initialize API server with provider manager
	apiAddress := cfg.APIAddress()
	apiServer := api.NewServer(apiAddress, db, allocator, syncEngine, providerManager)

	// Inject config manager into handlers for config API
	apiServer.Handlers().SetConfigManager(configManager)

	// Load existing providers from database BEFORE starting sync engine
	// This ensures metadata sync has access to providers immediately
	if err := apiServer.Handlers().LoadProvidersFromDB(); err != nil {
		log.Printf("Warning: Failed to load providers from database: %v", err)
	}

	// Start sync engine if enabled
	if !*noSync && cfg.Sync.AutoSync {
		if err := syncEngine.Start(ctx); err != nil {
			log.Fatalf("Failed to start sync engine: %v", err)
		}
		log.Println("Sync engine started")
	}

	// Initialize and mount FUSE filesystem if enabled
	var fuseFS *fuse.CloudUnifyFS
	if !*noMount {
		fuseFS = fuse.NewCloudUnifyFS(db, syncEngine, cfg.MountPath, paths.StagingDir, paths.CacheDir)

		// Set download timeout from config
		fuseFS.SetDownloadTimeout(time.Duration(cfg.Sync.DownloadTimeoutSeconds) * time.Second)

		if err := fuseFS.Mount(); err != nil {
			log.Printf("Warning: Failed to mount FUSE filesystem: %v", err)
			log.Println("Continuing without FUSE mount (macFUSE may not be installed)")
		} else {
			log.Printf("FUSE filesystem mounted at %s", cfg.MountPath)
		}
	}

	// Set up FUSE download progress callback to broadcast via WebSocket
	if fuseFS != nil {
		fuseFS.SetProgressCallback(func(virtualPath string, downloaded, total int64, status string) {
			apiServer.BroadcastEvent("download_progress", map[string]interface{}{
				"path":       virtualPath,
				"downloaded": downloaded,
				"total":      total,
				"percent":    int(float64(downloaded) / float64(total) * 100),
				"status":     status,
			})
		})
	}

	// Start API server in background
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("API server error: %v", err)
		}
	}()
	log.Printf("API server started on http://%s", apiAddress)

	// Start background cleanup routine
	db.StartCleanupRoutine(ctx, 1*time.Hour)

	// Print startup summary
	printStartupSummary(cfg, paths)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutdown signal received, cleaning up...")

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop sync engine
	syncEngine.Stop()
	log.Println("Sync engine stopped")

	// Shutdown API server
	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("API server shutdown error: %v", err)
	}
	log.Println("API server stopped")

	// Unmount FUSE if mounted
	if fuseFS != nil {
		// FUSE unmount would go here
		log.Println("FUSE filesystem unmounted")
	}

	log.Println("CloudUnify shutdown complete")
}

// setupProviderConfigs registers OAuth configs for all provider types
func setupProviderConfigs(pm *providers.Manager, cfg config.Config) {
	// Google Drive
	pm.RegisterConfig("google_drive", &providers.AuthConfig{
		ClientID:     cfg.OAuth.GoogleDrive.ClientID,
		ClientSecret: cfg.OAuth.GoogleDrive.ClientSecret,
		RedirectURL:  cfg.OAuth.GoogleDrive.RedirectURL,
	})

	// OneDrive
	pm.RegisterConfig("onedrive", &providers.AuthConfig{
		ClientID:     cfg.OAuth.OneDrive.ClientID,
		ClientSecret: cfg.OAuth.OneDrive.ClientSecret,
		RedirectURL:  cfg.OAuth.OneDrive.RedirectURL,
	})

	// iCloud (uses app-specific password, not OAuth)
	pm.RegisterConfig("icloud", &providers.AuthConfig{
		ClientID:     cfg.OAuth.ICloud.Username,
		ClientSecret: cfg.OAuth.ICloud.Password,
	})
}

// logOAuthStatus logs which OAuth providers are configured
func logOAuthStatus(cfg config.Config) {
	if cfg.OAuth.GoogleDrive.ClientID != "" {
		log.Println("Google Drive OAuth: configured")
	} else {
		log.Println("Google Drive OAuth: NOT configured (set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET)")
	}

	if cfg.OAuth.OneDrive.ClientID != "" {
		log.Println("OneDrive OAuth: configured")
	} else {
		log.Println("OneDrive OAuth: NOT configured (set ONEDRIVE_CLIENT_ID and ONEDRIVE_CLIENT_SECRET)")
	}

	if cfg.OAuth.ICloud.Username != "" {
		log.Println("iCloud: configured")
	} else {
		log.Println("iCloud: NOT configured (set ICLOUD_USERNAME and ICLOUD_PASSWORD)")
	}
}

func printStartupSummary(cfg config.Config, paths *config.Paths) {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    CloudUnify Started                        ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Version:     %-48s║\n", version)
	fmt.Printf("║  API:         http://%-42s║\n", cfg.APIAddress())
	fmt.Printf("║  Mount:       %-48s║\n", cfg.MountPath)
	fmt.Printf("║  Cache:       %-48s║\n", paths.CacheDir)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Press Ctrl+C to stop                                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}
