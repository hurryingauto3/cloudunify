package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

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
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"version": "0.1.0",
	})
}

// HandleVersion returns version information
func (h *Handlers) HandleVersion(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"version":    "0.1.0",
		"go_version": "1.21+",
		"build_time": "development",
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

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"provider":         provider,
		"is_authenticated": isAuth,
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
		respondJSON(w, http.StatusCreated, map[string]interface{}{
			"provider": provider,
			"message":  "Provider created, OAuth not available: " + err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"provider": provider,
		"auth_url": authURL,
		"state":    state,
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

	// Queue delete operation
	if _, err := h.syncEngine.EnqueueDelete(r.Context(), path, 0); err != nil {
		respondError(w, http.StatusInternalServerError, "SYNC_ERROR", "Failed to queue delete")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

// HandleSearchFiles searches for files
func (h *Handlers) HandleSearchFiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	// For now, just return empty results
	// TODO: Implement actual search
	respondJSON(w, http.StatusOK, []interface{}{})
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
			log.Printf("Loaded provider %s (ID: %d) from database", dbProvider.Name, dbProvider.ID)
		}
	}

	return nil
}
