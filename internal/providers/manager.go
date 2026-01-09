package providers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Manager manages cloud provider instances and OAuth flows
type Manager struct {
	mu        sync.RWMutex
	providers map[int64]CloudProvider
	configs   map[string]*AuthConfig // Provider type -> OAuth config
	pending   map[string]*PendingAuth // State -> pending auth info
}

// PendingAuth represents a pending OAuth authorization
type PendingAuth struct {
	ProviderID   int64
	ProviderType string
	State        string
	CreatedAt    time.Time
}

// NewManager creates a new provider manager
func NewManager() *Manager {
	return &Manager{
		providers: make(map[int64]CloudProvider),
		configs:   make(map[string]*AuthConfig),
		pending:   make(map[string]*PendingAuth),
	}
}

// RegisterConfig registers OAuth configuration for a provider type
func (m *Manager) RegisterConfig(providerType string, config *AuthConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.configs[providerType] = config
}

// GetConfig returns the OAuth config for a provider type
func (m *Manager) GetConfig(providerType string) *AuthConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.configs[providerType]
}

// HasValidConfig checks if a provider type has valid OAuth credentials configured
func (m *Manager) HasValidConfig(providerType string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, ok := m.configs[providerType]
	if !ok {
		return false
	}
	// For OAuth providers, check client ID is set
	if providerType == "google_drive" || providerType == "onedrive" {
		return config.ClientID != "" && config.ClientSecret != ""
	}
	// iCloud uses app-specific password
	return true
}

// CreateProvider creates a new provider instance
func (m *Manager) CreateProvider(providerType string, name string) (CloudProvider, error) {
	m.mu.RLock()
	config, ok := m.configs[providerType]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no configuration for provider type: %s", providerType)
	}

	var provider CloudProvider
	switch providerType {
	case "google_drive":
		provider = NewGoogleDriveProvider(name, config)
	case "onedrive":
		provider = NewOneDriveProvider(name, config)
	case "icloud":
		provider = NewICloudProvider(name, config)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}

	return provider, nil
}

// RegisterProvider registers a provider instance by database ID
func (m *Manager) RegisterProvider(id int64, provider CloudProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[id] = provider
}

// GetProvider returns a provider instance by database ID
func (m *Manager) GetProvider(id int64) CloudProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.providers[id]
}

// RemoveProvider removes a provider instance
func (m *Manager) RemoveProvider(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.providers, id)
}

// StartAuth starts an OAuth flow for a provider
func (m *Manager) StartAuth(providerID int64, providerType string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	config, ok := m.configs[providerType]
	if !ok {
		return "", "", fmt.Errorf("no configuration for provider type: %s", providerType)
	}

	// Generate random state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate state: %w", err)
	}
	state := hex.EncodeToString(stateBytes)

	// Store pending auth
	m.pending[state] = &PendingAuth{
		ProviderID:   providerID,
		ProviderType: providerType,
		State:        state,
		CreatedAt:    time.Now(),
	}

	// Create temporary provider to get auth URL
	var provider CloudProvider
	switch providerType {
	case "google_drive":
		provider = NewGoogleDriveProvider("temp", config)
	case "onedrive":
		provider = NewOneDriveProvider("temp", config)
	default:
		return "", "", fmt.Errorf("OAuth not supported for provider type: %s", providerType)
	}

	authURL := provider.GetAuthURL(state)
	return authURL, state, nil
}

// CompleteAuth completes an OAuth flow
func (m *Manager) CompleteAuth(ctx context.Context, state, code string) (*PendingAuth, *TokenInfo, error) {
	m.mu.Lock()
	pending, ok := m.pending[state]
	if !ok {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("invalid or expired state")
	}

	// Check if state is expired (10 minutes)
	if time.Since(pending.CreatedAt) > 10*time.Minute {
		delete(m.pending, state)
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("state expired")
	}

	config := m.configs[pending.ProviderType]
	delete(m.pending, state)
	m.mu.Unlock()

	// Create provider and exchange code
	var provider CloudProvider
	switch pending.ProviderType {
	case "google_drive":
		provider = NewGoogleDriveProvider("temp", config)
	case "onedrive":
		provider = NewOneDriveProvider("temp", config)
	default:
		return nil, nil, fmt.Errorf("OAuth not supported for provider type: %s", pending.ProviderType)
	}

	tokens, err := provider.ExchangeCode(ctx, code)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	return pending, tokens, nil
}

// CleanupExpiredPending removes expired pending authentications
func (m *Manager) CleanupExpiredPending() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for state, pending := range m.pending {
		if now.Sub(pending.CreatedAt) > 10*time.Minute {
			delete(m.pending, state)
		}
	}
}

// GetAllProviders returns all registered provider instances
func (m *Manager) GetAllProviders() map[int64]CloudProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[int64]CloudProvider, len(m.providers))
	for k, v := range m.providers {
		result[k] = v
	}
	return result
}
