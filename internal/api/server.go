package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	"cloudunify/internal/database"
	"cloudunify/internal/storage"
	"cloudunify/internal/sync"
)

// Server represents the HTTP API server
type Server struct {
	router     *mux.Router
	httpServer *http.Server
	handlers   *Handlers
	wsHub      *WSHub
	db         *database.DB
	allocator  *storage.Allocator
	syncEngine *sync.Engine
	address    string
}

// NewServer creates a new API server
func NewServer(address string, db *database.DB, allocator *storage.Allocator, syncEngine *sync.Engine) *Server {
	s := &Server{
		router:     mux.NewRouter(),
		db:         db,
		allocator:  allocator,
		syncEngine: syncEngine,
		address:    address,
	}

	s.handlers = NewHandlers(db, allocator, syncEngine)
	s.wsHub = NewWSHub()

	s.setupRoutes()

	return s
}

// setupRoutes configures all API routes
func (s *Server) setupRoutes() {
	// API router
	api := s.router.PathPrefix("/api").Subrouter()

	// Health and version
	api.HandleFunc("/health", s.handlers.HandleHealth).Methods("GET")
	api.HandleFunc("/version", s.handlers.HandleVersion).Methods("GET")

	// Provider management
	api.HandleFunc("/providers", s.handlers.HandleListProviders).Methods("GET")
	api.HandleFunc("/providers", s.handlers.HandleCreateProvider).Methods("POST")
	api.HandleFunc("/providers/{id}", s.handlers.HandleGetProvider).Methods("GET")
	api.HandleFunc("/providers/{id}", s.handlers.HandleDeleteProvider).Methods("DELETE")
	api.HandleFunc("/providers/{id}/quota", s.handlers.HandleGetProviderQuota).Methods("GET")

	// Storage information
	api.HandleFunc("/storage", s.handlers.HandleGetStorage).Methods("GET")
	api.HandleFunc("/storage/usage", s.handlers.HandleGetStorageUsage).Methods("GET")

	// File management
	api.HandleFunc("/files", s.handlers.HandleListFiles).Methods("GET")
	api.HandleFunc("/files/{path:.*}", s.handlers.HandleGetFile).Methods("GET")
	api.HandleFunc("/files/{path:.*}", s.handlers.HandleDeleteFile).Methods("DELETE")
	api.HandleFunc("/files/search", s.handlers.HandleSearchFiles).Methods("POST")

	// Sync operations
	api.HandleFunc("/sync/queue", s.handlers.HandleGetSyncQueue).Methods("GET")
	api.HandleFunc("/sync/status", s.handlers.HandleGetSyncStatus).Methods("GET")
	api.HandleFunc("/sync/pause", s.handlers.HandlePauseSync).Methods("POST")
	api.HandleFunc("/sync/resume", s.handlers.HandleResumeSync).Methods("POST")
	api.HandleFunc("/sync/queue/{id}", s.handlers.HandleCancelSyncItem).Methods("DELETE")

	// System
	api.HandleFunc("/shutdown", s.handlers.HandleShutdown).Methods("POST")

	// WebSocket
	s.router.HandleFunc("/ws", s.wsHub.HandleWebSocket)

	// Serve static files for the web UI (in production)
	// s.router.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/dist")))
}

// Start starts the HTTP server
func (s *Server) Start() error {
	// Configure CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:8080", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	})

	// Start WebSocket hub
	go s.wsHub.Run()

	// Bridge sync events to WebSocket
	s.wsHub.BridgeSyncEvents(s.syncEngine.Queue())

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.address,
		Handler:      c.Handler(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("API server starting on %s", s.address)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down API server...")
	return s.httpServer.Shutdown(ctx)
}

// Address returns the server address
func (s *Server) Address() string {
	return s.address
}

// WSHub returns the WebSocket hub
func (s *Server) WSHub() *WSHub {
	return s.wsHub
}

// Router returns the router for testing
func (s *Server) Router() *mux.Router {
	return s.router
}

// BroadcastEvent sends an event to all WebSocket clients
func (s *Server) BroadcastEvent(eventType string, payload interface{}) {
	s.wsHub.Broadcast(WSEvent{
		Type:    eventType,
		Payload: payload,
		Time:    time.Now(),
	})
}

// HealthCheck returns a simple health check handler
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok"}`)
}
