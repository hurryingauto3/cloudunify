package storage

import (
	"context"
	"errors"
	"sort"

	"cloudunify/internal/database"
)

// Common errors
var (
	ErrNoProviders       = errors.New("no providers available")
	ErrInsufficientSpace = errors.New("insufficient storage space across all providers")
)

// AllocationStrategy defines how files are distributed across providers
type AllocationStrategy string

const (
	// StrategyBalancedUsage distributes files to balance usage percentage across providers
	StrategyBalancedUsage AllocationStrategy = "balanced_usage"

	// StrategyMostFreeSpace places files on the provider with most free space
	StrategyMostFreeSpace AllocationStrategy = "most_free_space"

	// StrategyRoundRobin distributes files evenly across providers in rotation
	StrategyRoundRobin AllocationStrategy = "round_robin"
)

// Allocator handles storage allocation decisions
type Allocator struct {
	db              *database.DB
	strategy        AllocationStrategy
	roundRobinIndex int
}

// NewAllocator creates a new storage allocator
func NewAllocator(db *database.DB, strategy AllocationStrategy) *Allocator {
	return &Allocator{
		db:       db,
		strategy: strategy,
	}
}

// ChooseProvider selects the best provider for storing a file of the given size
func (a *Allocator) ChooseProvider(ctx context.Context, fileSize int64) (*database.Provider, error) {
	providers, err := a.db.ListEnabledProviders(ctx)
	if err != nil {
		return nil, err
	}

	if len(providers) == 0 {
		return nil, ErrNoProviders
	}

	// Filter providers with enough space
	var eligible []*database.Provider
	for _, p := range providers {
		if p.FreeBytes() >= fileSize {
			eligible = append(eligible, p)
		}
	}

	if len(eligible) == 0 {
		return nil, ErrInsufficientSpace
	}

	switch a.strategy {
	case StrategyBalancedUsage:
		return a.chooseByBalancedUsage(eligible)
	case StrategyMostFreeSpace:
		return a.chooseByMostFreeSpace(eligible)
	case StrategyRoundRobin:
		return a.chooseByRoundRobin(eligible)
	default:
		return a.chooseByBalancedUsage(eligible)
	}
}

// chooseByBalancedUsage selects the provider with the lowest usage percentage
func (a *Allocator) chooseByBalancedUsage(providers []*database.Provider) (*database.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoProviders
	}

	// Sort by usage percentage (ascending)
	sorted := make([]*database.Provider, len(providers))
	copy(sorted, providers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].UsagePercent() < sorted[j].UsagePercent()
	})

	return sorted[0], nil
}

// chooseByMostFreeSpace selects the provider with the most free space
func (a *Allocator) chooseByMostFreeSpace(providers []*database.Provider) (*database.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoProviders
	}

	var best *database.Provider
	var maxFree int64 = -1

	for _, p := range providers {
		if p.FreeBytes() > maxFree {
			maxFree = p.FreeBytes()
			best = p
		}
	}

	return best, nil
}

// chooseByRoundRobin cycles through providers in order
func (a *Allocator) chooseByRoundRobin(providers []*database.Provider) (*database.Provider, error) {
	if len(providers) == 0 {
		return nil, ErrNoProviders
	}

	// Ensure index is within bounds
	if a.roundRobinIndex >= len(providers) {
		a.roundRobinIndex = 0
	}

	selected := providers[a.roundRobinIndex]
	a.roundRobinIndex = (a.roundRobinIndex + 1) % len(providers)

	return selected, nil
}

// UpdateUsage updates the used bytes for a provider
func (a *Allocator) UpdateUsage(ctx context.Context, providerID int64, delta int64) error {
	provider, err := a.db.GetProvider(ctx, providerID)
	if err != nil {
		return err
	}
	if provider == nil {
		return errors.New("provider not found")
	}

	newUsed := provider.UsedBytes + delta
	if newUsed < 0 {
		newUsed = 0
	}

	return a.db.UpdateProviderUsage(ctx, providerID, newUsed)
}

// GetTotalStorage returns aggregated storage information across all providers
func (a *Allocator) GetTotalStorage(ctx context.Context) (*TotalStorage, error) {
	providers, err := a.db.ListEnabledProviders(ctx)
	if err != nil {
		return nil, err
	}

	var total TotalStorage
	for _, p := range providers {
		total.TotalBytes += p.QuotaBytes
		total.UsedBytes += p.UsedBytes
		total.Providers = append(total.Providers, ProviderStorage{
			ID:         p.ID,
			Name:       p.Name,
			Type:       string(p.Type),
			TotalBytes: p.QuotaBytes,
			UsedBytes:  p.UsedBytes,
			FreeBytes:  p.FreeBytes(),
		})
	}
	total.FreeBytes = total.TotalBytes - total.UsedBytes

	return &total, nil
}

// TotalStorage represents aggregated storage across all providers
type TotalStorage struct {
	TotalBytes int64             `json:"total_bytes"`
	UsedBytes  int64             `json:"used_bytes"`
	FreeBytes  int64             `json:"free_bytes"`
	Providers  []ProviderStorage `json:"providers"`
}

// ProviderStorage represents storage info for a single provider
type ProviderStorage struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	TotalBytes int64  `json:"total_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	FreeBytes  int64  `json:"free_bytes"`
}

// CanFit checks if a file of the given size can fit in any provider
func (a *Allocator) CanFit(ctx context.Context, fileSize int64) (bool, error) {
	providers, err := a.db.ListEnabledProviders(ctx)
	if err != nil {
		return false, err
	}

	for _, p := range providers {
		if p.FreeBytes() >= fileSize {
			return true, nil
		}
	}

	return false, nil
}

// SetStrategy changes the allocation strategy
func (a *Allocator) SetStrategy(strategy AllocationStrategy) {
	a.strategy = strategy
}
