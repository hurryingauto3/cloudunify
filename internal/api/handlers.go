package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"cloudunify/internal/config"
	"cloudunify/internal/database"
	"cloudunify/internal/providers"
	"cloudunify/internal/storage"
	"cloudunify/internal/sync"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	db              *database.DB
	allocator       *storage.Allocator
	syncEngine      *sync.Engine
	providerManager *providers.Manager
	configManager   *config.Manager
}

// NewHandlers creates a new handlers instance
func NewHandlers(db *database.DB, allocator *storage.Allocator, syncEngine *sync.Engine, providerManager *providers.Manager) *Handlers {
	return &Handlers{
		db:              db,
		allocator:       allocator,
		syncEngine:      syncEngine,
		providerManager: providerManager,
	}
}

// SetConfigManager sets the configuration manager (called after handlers are created)
func (h *Handlers) SetConfigManager(cm *config.Manager) {
	h.configManager = cm
}

// ErrorResponse represents an API error response
type ErrorResponse struct {
	Error struct {
		Code    string      `json:"code"`
		Message string      `json:"message"`
		Details interface{} `json:"details,omitempty"`
	} `json:"error"`
}

// respondJSON sends a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError sends an error response
func respondError(w http.ResponseWriter, status int, code, message string) {
	resp := ErrorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	respondJSON(w, status, resp)
}

// Health check handler
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}{
		Status:  "healthy",
		Version: "0.1.0",
	})
}

// HandleVersion returns version information
func (h *Handlers) HandleVersion(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, struct {
		Version   string `json:"version"`
		GoVersion string `json:"go_version"`
		BuildTime string `json:"build_time"`
	}{
		Version:   "0.1.0",
		GoVersion: "1.21+",
		BuildTime: "development",
	})
}

// Provider handlers

// HandleListProviders returns all providers
func (h *Handlers) HandleListProviders(w http.ResponseWriter, r *http.Request) {
	dbProviders, err := h.db.ListProviders(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list providers")
		return
	}

	// Add authentication status to response
	type ProviderResponse struct {
		*database.Provider
		IsAuthenticated bool `json:"is_authenticated"`
	}

	response := make([]ProviderResponse, len(dbProviders))
	for i, p := range dbProviders {
		isAuth := false
		if provider := h.providerManager.GetProvider(p.ID); provider != nil {
			isAuth = provider.IsAuthenticated()
		}
		response[i] = ProviderResponse{
			Provider:        p,
			IsAuthenticated: isAuth,
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// HandleGetProvider returns a single provider
func (h *Handlers) HandleGetProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

	provider, err := h.db.GetProvider(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get provider")
		return
	}
	if provider == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Provider not found")
		return
	}

	// Add authentication status
	isAuth := false
	if p := h.providerManager.GetProvider(id); p != nil {
		isAuth = p.IsAuthenticated()
	}

	respondJSON(w, http.StatusOK, struct {
		Provider        *database.Provider `json:"provider"`
		IsAuthenticated bool               `json:"is_authenticated"`
	}{
		Provider:        provider,
		IsAuthenticated: isAuth,
	})
}

// HandleCreateProvider creates a new provider and returns auth URL
func (h *Handlers) HandleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// If name is empty, use type as name
	if req.Name == "" {
		req.Name = req.Type
	}

	if req.Type == "" {
		respondError(w, http.StatusBadRequest, "MISSING_FIELDS", "Type is required")
		return
	}

	// Validate type
	validTypes := map[string]bool{
		"google_drive": true,
		"onedrive":     true,
		"icloud":       true,
	}
	if !validTypes[req.Type] {
		respondError(w, http.StatusBadRequest, "INVALID_TYPE", "Invalid provider type")
		return
	}

	// For iCloud, we use local folder approach - no OAuth needed
	if req.Type == "icloud" {
		// Create provider in database
		provider := &database.Provider{
			Name:    req.Name,
			Type:    database.ProviderType(req.Type),
			Enabled: false,
		}

		if err := h.db.CreateProvider(r.Context(), provider); err != nil {
			respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to create provider")
			return
		}

		// Return with special auth_url for iCloud local setup
		respondJSON(w, http.StatusCreated, struct {
			Provider *database.Provider `json:"provider"`
			AuthURL  string             `json:"auth_url"`
			State    string             `json:"state"`
			Local    bool               `json:"local"`
		}{
			Provider: provider,
			AuthURL:  fmt.Sprintf("cloudunify://icloud-local?provider_id=%d", provider.ID),
			State:    fmt.Sprintf("icloud_%d", provider.ID),
			Local:    true,
		})
		return
	}

	// Check if OAuth is configured for this provider type
	if !h.providerManager.HasValidConfig(req.Type) {
		respondError(w, http.StatusBadRequest, "OAUTH_NOT_CONFIGURED",
			"OAuth credentials not configured for "+req.Type+". Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET environment variables.")
		return
	}

	// Create provider in database
	provider := &database.Provider{
		Name:    req.Name,
		Type:    database.ProviderType(req.Type),
		Enabled: false, // Not enabled until authenticated
	}

	if err := h.db.CreateProvider(r.Context(), provider); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to create provider")
		return
	}

	// Start OAuth flow
	authURL, state, err := h.providerManager.StartAuth(provider.ID, req.Type)
	if err != nil {
		log.Printf("Failed to start OAuth: %v", err)
		// Still return success but without auth_url
		respondJSON(w, http.StatusCreated, struct {
			Provider *database.Provider `json:"provider"`
			Message  string             `json:"message"`
		}{
			Provider: provider,
			Message:  "Provider created, OAuth not available: " + err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, struct {
		Provider *database.Provider `json:"provider"`
		AuthURL  string             `json:"auth_url"`
		State    string             `json:"state"`
	}{
		Provider: provider,
		AuthURL:  authURL,
		State:    state,
	})
}

// HandleDeleteProvider deletes a provider
func (h *Handlers) HandleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

	// Remove from manager
	h.providerManager.RemoveProvider(id)

	// Unregister from sync engine
	h.syncEngine.UnregisterProvider(id)

	if err := h.db.DeleteProvider(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to delete provider")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleGetProviderQuota returns quota for a provider
func (h *Handlers) HandleGetProviderQuota(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

	dbProvider, err := h.db.GetProvider(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get provider")
		return
	}
	if dbProvider == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Provider not found")
		return
	}

	// Try to get live quota from provider
	provider := h.providerManager.GetProvider(id)
	if provider != nil && provider.IsAuthenticated() {
		quota, err := provider.GetQuota(r.Context())
		if err == nil {
			// Update database with fresh quota
			h.db.UpdateProviderUsage(r.Context(), id, quota.UsedBytes)
			respondJSON(w, http.StatusOK, quota)
			return
		}
		log.Printf("Failed to get live quota: %v", err)
	}

	// Fall back to database values
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total_bytes": dbProvider.QuotaBytes,
		"used_bytes":  dbProvider.UsedBytes,
		"free_bytes":  dbProvider.FreeBytes(),
	})
}

// OAuth handlers

// HandleGetAuthURL returns the OAuth URL for a provider type
func (h *Handlers) HandleGetAuthURL(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	providerType := vars["type"]

	// Validate type
	validTypes := map[string]bool{
		"google_drive": true,
		"onedrive":     true,
	}
	if !validTypes[providerType] {
		respondError(w, http.StatusBadRequest, "INVALID_TYPE", "Invalid provider type or OAuth not supported")
		return
	}

	if !h.providerManager.HasValidConfig(providerType) {
		respondError(w, http.StatusBadRequest, "OAUTH_NOT_CONFIGURED",
			"OAuth credentials not configured. Set environment variables for "+providerType)
		return
	}

	// Get provider ID from query if provided
	providerIDStr := r.URL.Query().Get("provider_id")
	var providerID int64
	if providerIDStr != "" {
		var err error
		providerID, err = strconv.ParseInt(providerIDStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
			return
		}
	}

	authURL, state, err := h.providerManager.StartAuth(providerID, providerType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "AUTH_ERROR", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleOAuthCallback handles the OAuth callback from providers
func (h *Handlers) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		// Redirect to frontend with error
		http.Redirect(w, r, "http://localhost:5173/setup?error="+errorParam+"&error_description="+errorDesc, http.StatusFound)
		return
	}

	if code == "" || state == "" {
		http.Redirect(w, r, "http://localhost:5173/setup?error=missing_params", http.StatusFound)
		return
	}

	// Complete the OAuth flow
	pending, tokens, err := h.providerManager.CompleteAuth(r.Context(), state, code)
	if err != nil {
		log.Printf("OAuth callback error: %v", err)
		http.Redirect(w, r, "http://localhost:5173/setup?error=auth_failed&message="+err.Error(), http.StatusFound)
		return
	}

	// Update provider in database with tokens
	dbProvider, err := h.db.GetProvider(r.Context(), pending.ProviderID)
	if err != nil || dbProvider == nil {
		log.Printf("Failed to get provider: %v", err)
		http.Redirect(w, r, "http://localhost:5173/setup?error=provider_not_found", http.StatusFound)
		return
	}

	// Store tokens in database
	dbProvider.AccessToken = tokens.AccessToken
	dbProvider.RefreshToken = tokens.RefreshToken
	dbProvider.TokenExpiry = &tokens.Expiry
	dbProvider.Enabled = true

	if err := h.db.UpdateProvider(r.Context(), dbProvider); err != nil {
		log.Printf("Failed to update provider: %v", err)
		http.Redirect(w, r, "http://localhost:5173/setup?error=save_failed", http.StatusFound)
		return
	}

	// Create and register the provider instance
	config := h.providerManager.GetConfig(pending.ProviderType)
	var provider providers.CloudProvider
	switch pending.ProviderType {
	case "google_drive":
		provider = providers.NewGoogleDriveProvider(dbProvider.Name, config)
	case "onedrive":
		provider = providers.NewOneDriveProvider(dbProvider.Name, config)
	}

	if provider != nil {
		provider.SetTokens(tokens)
		h.providerManager.RegisterProvider(dbProvider.ID, provider)
		// Also register with sync engine for file operations
		h.syncEngine.RegisterProvider(dbProvider.ID, provider)

		// Try to get quota and update database
		if quota, err := provider.GetQuota(r.Context()); err == nil {
			dbProvider.QuotaBytes = quota.TotalBytes
			dbProvider.UsedBytes = quota.UsedBytes
			h.db.UpdateProvider(r.Context(), dbProvider)
		}

		// Trigger background metadata sync for this provider
		go func(providerID int64) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			_ = h.syncEngine.SyncMetadata(ctx, providerID)
		}(dbProvider.ID)
	}

	// Redirect to frontend with success
	http.Redirect(w, r, "http://localhost:5173/setup?success=true&provider_id="+strconv.FormatInt(pending.ProviderID, 10), http.StatusFound)
}

// HandleRefreshToken refreshes the OAuth token for a provider
func (h *Handlers) HandleRefreshToken(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

	provider := h.providerManager.GetProvider(id)
	if provider == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Provider not found or not initialized")
		return
	}

	tokens, err := provider.RefreshToken(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "REFRESH_ERROR", err.Error())
		return
	}

	// Update database
	dbProvider, _ := h.db.GetProvider(r.Context(), id)
	if dbProvider != nil {
		dbProvider.AccessToken = tokens.AccessToken
		dbProvider.TokenExpiry = &tokens.Expiry
		h.db.UpdateProvider(r.Context(), dbProvider)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":      true,
		"token_expiry": tokens.Expiry,
	})
}

// HandleVerifyICloud verifies and completes iCloud local folder setup
func (h *Handlers) HandleVerifyICloud(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

	var req struct {
		CustomPath string `json:"custom_path,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Get the provider from database
	dbProvider, err := h.db.GetProvider(r.Context(), id)
	if err != nil || dbProvider == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Provider not found")
		return
	}

	if dbProvider.Type != database.ProviderICloud {
		respondError(w, http.StatusBadRequest, "INVALID_TYPE", "Provider is not iCloud")
		return
	}

	// Create iCloud provider and verify local folder
	config := h.providerManager.GetConfig("icloud")
	icloudProvider := providers.NewICloudProvider(dbProvider.Name, config)

	// ExchangeCode verifies the local folder exists and is writable
	code := "local"
	if req.CustomPath != "" {
		code = req.CustomPath
	}

	tokens, err := icloudProvider.ExchangeCode(r.Context(), code)
	if err != nil {
		respondError(w, http.StatusBadRequest, "VERIFICATION_FAILED", err.Error())
		return
	}

	// Set tokens on provider
	icloudProvider.SetTokens(tokens)

	// Update provider in database
	dbProvider.AccessToken = tokens.AccessToken // This contains the base path
	dbProvider.TokenExpiry = &tokens.Expiry
	dbProvider.Enabled = true

	if err := h.db.UpdateProvider(r.Context(), dbProvider); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to update provider")
		return
	}

	// Register provider
	h.providerManager.RegisterProvider(dbProvider.ID, icloudProvider)
	h.syncEngine.RegisterProvider(dbProvider.ID, icloudProvider)

	// Get quota
	if quota, err := icloudProvider.GetQuota(r.Context()); err == nil {
		dbProvider.QuotaBytes = quota.TotalBytes
		dbProvider.UsedBytes = quota.UsedBytes
		h.db.UpdateProvider(r.Context(), dbProvider)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"provider": dbProvider,
		"message":  fmt.Sprintf("iCloud connected via local folder: %s", tokens.AccessToken),
	})
}

// Storage handlers

// HandleGetStorage returns aggregated storage stats
func (h *Handlers) HandleGetStorage(w http.ResponseWriter, r *http.Request) {
	storage, err := h.allocator.GetTotalStorage(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get storage info")
		return
	}
	respondJSON(w, http.StatusOK, storage)
}

// HandleGetStorageUsage returns usage breakdown by provider
func (h *Handlers) HandleGetStorageUsage(w http.ResponseWriter, r *http.Request) {
	summary, err := h.db.GetStorageSummary(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get storage summary")
		return
	}
	respondJSON(w, http.StatusOK, summary)
}

// File handlers

// HandleListFiles returns files, with optional path filter
func (h *Handlers) HandleListFiles(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
	}

	files, err := h.db.ListFilesInDirectory(r.Context(), path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list files")
		return
	}

	respondJSON(w, http.StatusOK, files)
}

// HandleGetFile returns file metadata
func (h *Handlers) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	path := "/" + mux.Vars(r)["path"]

	file, err := h.db.GetFileByPath(r.Context(), path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get file")
		return
	}
	if file == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "File not found")
		return
	}

	respondJSON(w, http.StatusOK, file)
}

// HandleDeleteFile deletes a file
func (h *Handlers) HandleDeleteFile(w http.ResponseWriter, r *http.Request) {
	path := "/" + mux.Vars(r)["path"]

	file, err := h.db.GetFileByPath(r.Context(), path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get file")
		return
	}
	if file == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "File not found")
		return
	}

	// Capture file info before deletion
	cloudFileID := file.CloudFileID
	providerID := file.ProviderID

	// Delete from database immediately
	if err := h.db.DeleteFile(r.Context(), file.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to delete file record")
		return
	}

	// Queue cloud delete operation with file info
	if _, err := h.syncEngine.EnqueueDelete(r.Context(), path, cloudFileID, providerID, 0); err != nil {
		respondError(w, http.StatusInternalServerError, "SYNC_ERROR", "Failed to queue delete")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// HandleSearchFiles searches for files
func (h *Handlers) HandleSearchFiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Query == "" {
		respondError(w, http.StatusBadRequest, "EMPTY_QUERY", "Search query is required")
		return
	}

	if req.Limit <= 0 {
		req.Limit = 50
	}

	files, err := h.db.SearchFiles(r.Context(), req.Query, req.Limit)
	if err != nil {
		log.Printf("Search error: %v", err)
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to search files")
		return
	}

	respondJSON(w, http.StatusOK, files)
}

// HandleUploadFile handles file uploads
func (h *Handlers) HandleUploadFile(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form - max 32MB in memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to parse multipart form: "+err.Error())
		return
	}

	// Get the file from the form
	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "No file provided: "+err.Error())
		return
	}
	defer file.Close()

	// Get remote path (optional, defaults to filename)
	remotePath := r.FormValue("path")
	if remotePath == "" {
		remotePath = "/" + header.Filename
	}

	// Get provider ID (optional - if not specified, use allocator)
	providerIDStr := r.FormValue("provider_id")
	var providerID int64
	if providerIDStr != "" {
		id, _ := strconv.Atoi(providerIDStr)
		providerID = int64(id)
	}

	// If no specific provider, find one with space using the allocator
	if providerID == 0 {
		// Get providers from database
		dbProviders, err := h.db.ListProviders(r.Context())
		if err != nil {
			respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list providers")
			return
		}

		// Find an authenticated provider with space
		for _, dbProvider := range dbProviders {
			if dbProvider.Enabled {
				if provider := h.providerManager.GetProvider(dbProvider.ID); provider != nil && provider.IsAuthenticated() {
					providerID = dbProvider.ID
					break
				}
			}
		}
	}

	if providerID == 0 {
		respondError(w, http.StatusBadRequest, "NO_PROVIDER", "No authenticated provider available")
		return
	}

	// Get the provider
	provider := h.providerManager.GetProvider(providerID)
	if provider == nil {
		respondError(w, http.StatusNotFound, "PROVIDER_NOT_FOUND", "Provider not found")
		return
	}
	if !provider.IsAuthenticated() {
		respondError(w, http.StatusUnauthorized, "NOT_AUTHENTICATED", "Provider not authenticated")
		return
	}

	// Upload the file
	ctx := r.Context()
	fileMeta, err := provider.UploadStream(ctx, file, remotePath, header.Size)
	if err != nil {
		log.Printf("Upload failed: %v", err)
		respondError(w, http.StatusInternalServerError, "UPLOAD_FAILED", "Upload failed: "+err.Error())
		return
	}

	// Store file metadata in database
	dbFile := &database.File{
		VirtualPath: remotePath,
		ProviderID:  providerID,
		CloudFileID: fileMeta.ID,
		CloudPath:   remotePath,
		SizeBytes:   header.Size,
		MimeType:    header.Header.Get("Content-Type"),
		IsDir:       false,
		Status:      database.FileStatusSynced,
	}

	if err := h.db.CreateFile(ctx, dbFile); err != nil {
		log.Printf("Failed to save file metadata: %v", err)
		// Don't fail the request - file is uploaded
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"success": true,
		"file": map[string]interface{}{
			"id":          fileMeta.ID,
			"name":        fileMeta.Name,
			"path":        remotePath,
			"size":        header.Size,
			"mime_type":   fileMeta.MimeType,
			"provider_id": providerID,
		},
	})
}

// Sync handlers

// HandleGetSyncQueue returns the sync queue
func (h *Handlers) HandleGetSyncQueue(w http.ResponseWriter, r *http.Request) {
	status := database.SyncStatus(r.URL.Query().Get("status"))

	items, err := h.syncEngine.Queue().List(r.Context(), status)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get sync queue")
		return
	}

	respondJSON(w, http.StatusOK, items)
}

// HandleGetSyncStatus returns current sync status
func (h *Handlers) HandleGetSyncStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := h.syncEngine.Queue().Stats(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get sync status")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"running": h.syncEngine.IsRunning(),
		"stats":   stats,
		"active":  h.syncEngine.Queue().GetActive(),
	})
}

// HandlePauseSync pauses sync operations
func (h *Handlers) HandlePauseSync(w http.ResponseWriter, r *http.Request) {
	h.syncEngine.Queue().Pause()
	respondJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

// HandleResumeSync resumes sync operations
func (h *Handlers) HandleResumeSync(w http.ResponseWriter, r *http.Request) {
	h.syncEngine.Queue().Resume()
	respondJSON(w, http.StatusOK, map[string]string{"status": "resumed"})
}

// HandleCancelSyncItem cancels a sync queue item
func (h *Handlers) HandleCancelSyncItem(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid queue item ID")
		return
	}

	if err := h.syncEngine.Queue().Cancel(r.Context(), id); err != nil {
		respondError(w, http.StatusInternalServerError, "SYNC_ERROR", "Failed to cancel sync item")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleShutdown initiates graceful shutdown
func (h *Handlers) HandleShutdown(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "shutting_down"})
	// Actual shutdown would be handled by the server
}

// HandleOAuthStatus returns the OAuth configuration status for all providers
func (h *Handlers) HandleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"google_drive": map[string]interface{}{
			"configured": h.providerManager.HasValidConfig("google_drive"),
		},
		"onedrive": map[string]interface{}{
			"configured": h.providerManager.HasValidConfig("onedrive"),
		},
		"icloud": map[string]interface{}{
			"configured": true, // iCloud uses app-specific password, always "configured"
		},
	}
	respondJSON(w, http.StatusOK, status)
}

// LoadProvidersFromDB loads existing providers from database and initializes them
func (h *Handlers) LoadProvidersFromDB() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbProviders, err := h.db.ListProviders(ctx)
	if err != nil {
		return err
	}

	for _, dbProvider := range dbProviders {
		if dbProvider.AccessToken == "" {
			continue // Skip providers without tokens
		}

		config := h.providerManager.GetConfig(string(dbProvider.Type))
		if config == nil {
			continue
		}

		var provider providers.CloudProvider
		switch dbProvider.Type {
		case database.ProviderGoogleDrive:
			provider = providers.NewGoogleDriveProvider(dbProvider.Name, config)
		case database.ProviderOneDrive:
			provider = providers.NewOneDriveProvider(dbProvider.Name, config)
		case database.ProviderICloud:
			provider = providers.NewICloudProvider(dbProvider.Name, config)
		}

		if provider != nil {
			tokens := &providers.TokenInfo{
				AccessToken:  dbProvider.AccessToken,
				RefreshToken: dbProvider.RefreshToken,
			}
			if dbProvider.TokenExpiry != nil {
				tokens.Expiry = *dbProvider.TokenExpiry
			}
			provider.SetTokens(tokens)
			h.providerManager.RegisterProvider(dbProvider.ID, provider)
			// Also register with sync engine for file operations
			h.syncEngine.RegisterProvider(dbProvider.ID, provider)
		}
	}

	return nil
}

// HandleGetConfig returns the current configuration
func (h *Handlers) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	if h.configManager == nil {
		respondError(w, http.StatusInternalServerError, "CONFIG_ERROR", "Configuration manager not initialized")
		return
	}

	cfg := h.configManager.Get()

	// Return a safe subset of config (no OAuth secrets)
	response := struct {
		MountPath          string             `json:"mount_path"`
		Cache              config.CacheConfig `json:"cache"`
		Sync               config.SyncConfig  `json:"sync"`
		AllocationStrategy string             `json:"allocation_strategy"`
	}{
		MountPath:          cfg.MountPath,
		Cache:              cfg.Cache,
		Sync:               cfg.Sync,
		AllocationStrategy: cfg.AllocationStrategy,
	}

	respondJSON(w, http.StatusOK, response)
}

// HandleUpdateConfig updates the configuration
func (h *Handlers) HandleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.configManager == nil {
		respondError(w, http.StatusInternalServerError, "CONFIG_ERROR", "Configuration manager not initialized")
		return
	}

	var req struct {
		Sync *config.SyncConfig `json:"sync,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	var restartRequired bool

	if req.Sync != nil {
		restartRequired = h.configManager.UpdateSyncConfig(*req.Sync)

		// Update sync engine with new config (live reload)
		if h.syncEngine != nil {
			h.syncEngine.UpdateConfig(h.configManager.GetSyncConfig())
		}
	}

	// Save config to disk
	if err := h.configManager.Save(); err != nil {
		log.Printf("Warning: Failed to save config: %v", err)
	}

	respondJSON(w, http.StatusOK, struct {
		Success         bool `json:"success"`
		RestartRequired bool `json:"restart_required"`
	}{
		Success:         true,
		RestartRequired: restartRequired,
	})
}

// HandlePinFile pins a file to cache and triggers download if not cached
func (h *Handlers) HandlePinFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid_id", "Invalid file ID")
		return
	}

	// Get file info
	file, err := h.db.GetFile(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to get file")
		return
	}
	if file == nil {
		respondError(w, http.StatusNotFound, "not_found", "File not found")
		return
	}

	// Update pinned status
	if err := h.db.UpdateFilePinned(r.Context(), id, true); err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to pin file")
		return
	}

	// Check if file is already cached
	isCached, _, err := h.db.GetFileCachedStatus(r.Context(), id)
	if err != nil {
		log.Printf("Warning: failed to check cache status: %v", err)
	}

	// If not cached, queue a download
	downloading := false
	if !isCached && file.CloudFileID != "" {
		_, err := h.syncEngine.EnqueueDownload(r.Context(), file.VirtualPath, "", 5) // High priority, empty localPath means use cache
		if err != nil {
			log.Printf("Warning: failed to queue download for pinned file: %v", err)
		} else {
			downloading = true
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "pinned",
		"is_cached":   isCached,
		"downloading": downloading,
	})
}

// HandleUnpinFile unpins a file from cache
func (h *Handlers) HandleUnpinFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid_id", "Invalid file ID")
		return
	}

	if err := h.db.UpdateFilePinned(r.Context(), id, false); err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to unpin file")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "unpinned"})
}

// HandleDehydrateFile removes a file from local cache (keeps cloud copy)
func (h *Handlers) HandleDehydrateFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid_id", "Invalid file ID")
		return
	}

	// Get file info
	file, err := h.db.GetFile(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to get file")
		return
	}
	if file == nil {
		respondError(w, http.StatusNotFound, "not_found", "File not found")
		return
	}

	// Don't allow dehydrating pinned files
	if file.Pinned {
		respondError(w, http.StatusBadRequest, "file_pinned", "Cannot dehydrate pinned file. Unpin first.")
		return
	}

	// Get and remove cache entry
	cacheEntry, err := h.db.GetCacheEntry(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to check cache")
		return
	}

	if cacheEntry == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"status":  "already_dehydrated",
			"message": "File is not in local cache",
		})
		return
	}

	// Delete local cache file
	if err := os.Remove(cacheEntry.LocalPath); err != nil && !os.IsNotExist(err) {
		log.Printf("Warning: failed to delete cache file: %v", err)
	}

	// Delete cache entry from database
	if err := h.db.DeleteCacheEntry(r.Context(), cacheEntry.ID); err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", "Failed to remove cache entry")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "dehydrated",
		"freed_bytes": cacheEntry.SizeBytes,
	})
}

// HandleDebugDBCounts returns file counts by provider for debugging
func (h *Handlers) HandleDebugDBCounts(w http.ResponseWriter, r *http.Request) {
	counts, err := h.db.CountFilesByProvider(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", fmt.Sprintf("Failed to get counts: %v", err))
		return
	}

	// Also get provider info
	providers, err := h.db.ListProviders(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", fmt.Sprintf("Failed to list providers: %v", err))
		return
	}

	result := make(map[string]interface{})
	result["total_files"] = 0

	providerData := make([]map[string]interface{}, 0)
	for _, p := range providers {
		count := counts[p.ID]
		result["total_files"] = result["total_files"].(int) + count
		providerData = append(providerData, map[string]interface{}{
			"id":           p.ID,
			"name":         p.Name,
			"type":         p.Type,
			"display_name": p.Type.DisplayName(),
			"enabled":      p.Enabled,
			"file_count":   count,
			"quota_bytes":  p.QuotaBytes,
			"used_bytes":   p.UsedBytes,
		})
	}
	result["providers"] = providerData

	respondJSON(w, http.StatusOK, result)
}

// HandleDebugTriggerIndex triggers metadata sync for a specific provider
func (h *Handlers) HandleDebugTriggerIndex(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	providerID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	// Verify provider exists
	provider, err := h.db.GetProvider(r.Context(), providerID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", fmt.Sprintf("Failed to get provider: %v", err))
		return
	}
	if provider == nil {
		respondError(w, http.StatusNotFound, "not_found", "Provider not found")
		return
	}

	// Trigger metadata sync in background
	go func(pID int64) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		_ = h.syncEngine.SyncMetadata(ctx, pID)
	}(providerID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "started",
		"provider_id": providerID,
		"message":     "Metadata sync started in background. Check logs for progress.",
	})
}

// HandleDebugListProviderFiles lists files for a specific provider (for debugging)
func (h *Handlers) HandleDebugListProviderFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr := vars["id"]
	providerID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid_id", "Invalid provider ID")
		return
	}

	// Get path from query param, default to root
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		dirPath = "/"
	}

	files, err := h.db.ListFilesInProviderDirectory(r.Context(), providerID, dirPath)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "db_error", fmt.Sprintf("Failed to list files: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"provider_id": providerID,
		"path":        dirPath,
		"count":       len(files),
		"files":       files,
	})
}
