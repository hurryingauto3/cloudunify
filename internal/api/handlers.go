package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	"cloudunify/internal/database"
	"cloudunify/internal/storage"
	"cloudunify/internal/sync"
)

// Handlers contains all HTTP handlers
type Handlers struct {
	db         *database.DB
	allocator  *storage.Allocator
	syncEngine *sync.Engine
}

// NewHandlers creates a new handlers instance
func NewHandlers(db *database.DB, allocator *storage.Allocator, syncEngine *sync.Engine) *Handlers {
	return &Handlers{
		db:         db,
		allocator:  allocator,
		syncEngine: syncEngine,
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
	providers, err := h.db.ListProviders(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to list providers")
		return
	}
	respondJSON(w, http.StatusOK, providers)
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

	respondJSON(w, http.StatusOK, provider)
}

// HandleCreateProvider creates a new provider
func (h *Handlers) HandleCreateProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if req.Name == "" || req.Type == "" {
		respondError(w, http.StatusBadRequest, "MISSING_FIELDS", "Name and type are required")
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

	provider := &database.Provider{
		Name:    req.Name,
		Type:    database.ProviderType(req.Type),
		Enabled: true,
	}

	if err := h.db.CreateProvider(r.Context(), provider); err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to create provider")
		return
	}

	respondJSON(w, http.StatusCreated, provider)
}

// HandleDeleteProvider deletes a provider
func (h *Handlers) HandleDeleteProvider(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.ParseInt(vars["id"], 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid provider ID")
		return
	}

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

	provider, err := h.db.GetProvider(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "DB_ERROR", "Failed to get provider")
		return
	}
	if provider == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Provider not found")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"total_bytes": provider.QuotaBytes,
		"used_bytes":  provider.UsedBytes,
		"free_bytes":  provider.FreeBytes(),
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
