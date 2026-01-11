# CloudUnify - Unified Cloud Storage System

## Project Overview
A cross-platform virtual filesystem that unifies multiple cloud storage providers (iCloud, Google Drive, OneDrive) into a single mountable drive with automatic syncing and intelligent file distribution.

**Target Platforms:** macOS, Windows, Linux
**Primary Use Case:** Large video file storage and management
**Total Storage Capacity:** 5TB (2TB iCloud + 2TB Google Drive + 1TB OneDrive)

---

## Implementation Status

**Last Updated:** 2026-01-11  
**Current Phase:** Phase 1 (Daily Driver)

### Completed
- FUSE virtual filesystem mounted at `~/CloudUnify`
- Drag-and-drop file upload via Finder and terminal
- Google Drive OAuth 2.0 authentication
- Multipart upload (≤5MB) and resumable upload (>5MB)
- Background sync engine (3 upload workers, 5 download workers)
- SQLite database for metadata
- REST API on port 8080
- WebSocket for real-time progress updates
- React dashboard with Vite
- **Lazy/deferred download** (files download only when content is read, not on Open)
- **Provider deletion safety** (sync engine cleanup before DB delete)
- **Metadata sync from Google Drive** (background indexing)
- **Storage allocator** (balanced usage policy)
- **Quota-aware placement**
- **Pin/unpin with cache protection** (pinned files never evicted from cache)
- **Dehydrate/free up space** (remove local copy, keep cloud-only placeholder)
- **UI search with 300ms debounce** (search files across all providers)
- **Range read support for Google Drive** (byte-range HTTP requests for video seeking)
- **Context menu in file browser** (right-click for Pin, Unpin, Dehydrate, Delete)

### In Progress (Phase 2)
- OneDrive integration
- iCloud integration
- Menubar/tray app
- Finder badges + context menu (macOS)

### Planned (Phase 3+)
- Cross-platform packaging
- Redundancy / mirroring
- Conflict resolution UI

---

## Product Vision

CloudUnify is a cross-provider "virtual disk + policy engine":

- **Namespace unification:** One tree, many providers
- **Hydration:** Online-only placeholders  ->  download on open/seek
- **Write path:** Staging  ->  policy decides placement  ->  background upload
- **Policy:** Allocation, redundancy, retention, cache, priorities
- **OS integration:** Status badges, context menus, background daemon, notifications

---

## Feature Inventory

### A) Unified Namespace & File Model

| # | Feature | Description |
|---|---------|-------------|
| A1 | Unified root with provider folders + merged view | **Mode 1:** `/CloudUnify/{Google Drive, OneDrive, iCloud}/…` (transparent) <br> **Mode 2:** Merged `/CloudUnify/…` with a single virtual tree (long-term) |
| A2 | Conflict model | Name collisions (same path exists on two providers) <br> Deterministic resolution: `prefer_local`, `prefer_provider`, `create_conflict_copy` |
| A3 | Stable IDs | Maintain mapping `virtual_path ↔ provider_id + cloud_file_id` <br> Survive rename/move without losing identity |
| A4 | Metadata normalization | modtime, size, mime, checksum, etag/revision, sharing bits (optional) |
| A5 | Directory listing performance | Cached listings, incremental refresh, pagination, "partial tree hydration" |

### B) Hydration, Placeholders & "Online-Only" Semantics (OneDrive-like behavior)

| # | Feature | Description |
|---|---------|-------------|
| B1 | Placeholder files | File exists in Finder but content is cloud-only until opened |
| B2 | Hydration on demand | Open/read triggers download; range reads for video seeking |
| B3 | Dehydration / "Free up space" | Convert local to online-only while keeping placeholder |
| B4 | Pin / "Always keep on this device" | "Pinned" files never evicted |
| B5 | Finder indicators + context menu | **Badges:** online-only / downloading / local / syncing / error <br> **Right-click actions:** "Download now", "Free up space", "Pin", "View versions", "Retry" |
| B6 | Platform-native implementation | **macOS:** Finder Sync Extension or File Provider extension <br> **Windows:** Cloud Files API for placeholders <br> **Linux:** Best-effort via FUSE + xattrs + tray UI (no first-class badges) |

### C) Sync Engine & Queue Control

| # | Feature | Description |
|---|---------|-------------|
| C1 | User-visible job queue | Upload/download/delete as jobs |
| C2 | Priority + sequencing | "Do these next" (manual reordering) <br> Rules: foreground hydration jobs > uploads > background indexing |
| C3 | Bandwidth and concurrency controls | Caps, schedules ("only overnight"), per-provider concurrency |
| C4 | Retry policy & failure classification | Retriable (rate limit/network) vs permanent (auth/quota) |
| C5 | Transactional correctness | DB-first state transitions; idempotent jobs; safe restarts |
| C6 | Progress + resumability | Resumable upload (Drive), resumable download where possible, checkpointed ranges |

### D) Multi-Cloud Allocation + Redundancy

| # | Feature | Description |
|---|---------|-------------|
| D1 | Placement policies | Balanced usage (default), cheapest/most-free, provider affinity by folder (e.g., /Movies  ->  Drive), file-type policy |
| D2 | Redundancy modes | None (single provider), Mirror x2 (two providers), Mirror xN (all selected providers) |
| D3 | Consistency model for redundancy | Write once  ->  fan-out uploads; atomic virtual commit after quorum <br> Read preference: nearest cached / healthiest provider |
| D4 | Repair tool | Detect missing replica and re-seed |
| D5 | User controls | Per-folder redundancy settings (scalable UX) |

### E) Caching for Large Video Workflows

| # | Feature | Description |
|---|---------|-------------|
| E1 | Range cache | Cache chunks; optimize for sequential reads + random seeks |
| E2 | Read-ahead / prefetch | Heuristics for players (VLC/QuickTime) seeking patterns |
| E3 | Adaptive cache sizing | Global cap + per-folder caps; LRU + pinned exemption |
| E4 | Integrity verification | Checksum on upload/download, background scrubber |
| E5 | Local staging optimization | "Fast writes" by staging + later streaming upload |

### F) Provider Integrations (Beyond Basic CRUD)

#### Google Drive
- Shared drives support
- Resumable uploads
- Delta-like changes (Drive "changes" API)

#### OneDrive
- Delta queries for listing
- Large file upload sessions
- Business vs personal accounts

#### iCloud
- Realistic plan: macOS-native integration first (local iCloud Drive folder mapping)
- CloudKit/WebDAV only if accepting limitations

### G) Background Daemon / "OS Native" Behavior

| # | Feature | Description |
|---|---------|-------------|
| G1 | Runs at login | Auto-start on system boot |
| G2 | Menu bar / system tray | Status: syncing, paused, errors, quotas, queue |
| G3 | Notifications | Failures, quota warnings, completed large uploads, auth expired |
| G4 | Auto-update | Self-updating mechanism |
| G5 | Crash safety | Watchdog, safe shutdown, DB recovery |

### H) Web Dashboard (and/or Local UI)

| # | Feature | Description |
|---|---------|-------------|
| H1 | Storage overview | Total + breakdown by provider |
| H2 | Queue management | Reorder, pause, retry operations |
| H3 | File browser | Search, filter, pinned, online-only indicators |
| H4 | Provider management | OAuth, quotas, disable provider |
| H5 | Policies editor | Allocation + redundancy + cache settings |

### I) Security & Privacy

| # | Feature | Description |
|---|---------|-------------|
| I1 | Token storage | Keychain (macOS) / Credential Manager (Windows) / Secret Service (Linux) |
| I2 | Per-file encryption | Optional v2 feature |
| I3 | Sensitive logging rules | Redaction of tokens and personal data |
| I4 | Least-privilege OAuth scopes | Minimal required permissions |
| I5 | Secure config backup | Export/import with encryption |

### J) Ops & Diagnostics

| # | Feature | Description |
|---|---------|-------------|
| J1 | Structured logs + log viewer | Filterable, searchable logs |
| J2 | "Collect diagnostics" bundle | One-click export for support |
| J3 | Health endpoints | API status checks |
| J4 | Self-test | Provider auth, upload/download, mount, placeholder behavior |
| J5 | Performance profiling | pprof + benchmarks |

---

## Implementation Roadmap

### Phase 0 — Hardening the Core COMPLETE

**Focus:** Reliability and foundation

- [x] Fix DB nullability issues
- [x] Deterministic job state machine (provider deletion cleanup)
- [x] Reliable upload pipeline for Drive (resumable)
- [x] Solid file mapping (`virtual_path ↔ cloud_id`)
- [x] Basic UI queue visibility

**Deliverable:** "Drop file  ->  upload reliably; restart doesn't break; queue visible"  
**Status:** Complete as of 2026-01-11

---

### Phase 1 — Usable as a Daily Driver *(Current Phase — 40%)*

**Focus:** Single provider + on-demand reads

- [x] Download-on-open + cache (lazy/deferred download implemented 2026-01-11)
- [ ] Range reads for large video seeking 
- [ ] Pin/unpin + "free up space" (dehydrate)
- [x] Search (by path/name) from DB (API exists, UI partial)


**Deliverable:** "It behaves like a cloud drive, not a sync toy"

---

### Phase 2 — Unified Browse Across Providers *(30%)*

**Focus:** Multi-provider namespace

- [x] Provider folders + merged view option (single merged tree exists)
- [ ] OneDrive integration (stub only)
- [ ] iCloud integration (stub only)
- [ ] Cross-provider move/copy semantics (defined rules)
- [x] Allocation policy (balanced usage)
- [x] Quota-aware placement

**Deliverable:** "3TB pooled namespace, seamless browsing"

---

### Phase 3 — Native OS Indicators *(The "Feels Legit" Milestone)*

**Focus:** OneDrive-like Finder status

- [ ] Basic menubar/tray with pause/resume
- [ ] macOS: Finder Sync extension badges + context menu
- [ ] Windows: Cloud Files API (placeholders + badges)
- [ ] Linux: Tray + extended attributes (best-effort)

**Deliverable:** "Finder shows online-only/local/syncing/error"

---

### Phase 4 — Redundancy + Policy UI

**Focus:** Replication feature

- [ ] Per-folder redundancy rules
- [ ] Mirror x2/xN + repair
- [ ] Read preference + health monitoring
- [ ] Policy editor in UI

**Deliverable:** "Critical folders have replicas; user understands it"

---

### Phase 5 — Reliability + Packaging + Updates

**Focus:** Ship-ready

- [ ] Installers, auto-update, signed builds
- [ ] Robust diagnostics bundle
- [ ] Stress tests (10GB+ videos, power loss, token expiry)

**Deliverable:** "Shippable product"

---

## Critical Success Features

The 5 "must-have" features to match user expectations from OneDrive/Drive desktop:

1. **Online-only placeholders + hydration**
2. **Pin / Free up space**
3. **Badges + context menu**
4. **Delta sync** (fast listings, quick consistency)
5. **Clear queue + retries** (user trust)

---

## Architecture Decision: FUSE vs Native

### Option A: Keep FUSE as Core (Fastest to Ship Cross-Platform)

| Pros | Cons |
|------|------|
| Cross-platform now | OS-native placeholders/badges are harder |
| Single codebase | Feels less native |
| Proven technology | Some performance overhead |

### Option B: Go "File Provider / Cloud Files API" Per OS (Most Native)

| Pros | Cons |
|------|------|
| Real placeholders + badges with OS semantics | More platform-specific engineering |
| Best user experience | Longer development time |
| Native performance | Three separate implementations |

### Recommended: Hybrid Path

- **Phase 0–2:** FUSE core for portability
- **Phase 3+:** Add native layers where they matter (macOS/Windows)

This allows rapid iteration on core functionality while building toward the native experience users expect.

---

## Technical Stack

### Core Technology
- **Language:** Go 1.21+
- **Virtual Filesystem:** cgofuse (cross-platform FUSE wrapper)
  - macOS: macFUSE
  - Linux: FUSE
  - Windows: WinFsp
- **Database:** SQLite (embedded, single file)
- **Frontend:** Web UI (React + Vite) served locally
- **API:** RESTful + WebSocket (for real-time updates)

### Key Dependencies
```
- github.com/winfsp/cgofuse (FUSE abstraction)
- github.com/mattn/go-sqlite3 (database)
- golang.org/x/oauth2 (OAuth flows)
- google.golang.org/api/drive/v3 (Google Drive API)
- github.com/Azure/azure-sdk-for-go (OneDrive via Graph API)
- github.com/gorilla/mux (HTTP router)
- github.com/gorilla/websocket (real-time updates)
- github.com/rs/cors (CORS handling)
```

### Build & Distribution
- **Build Tool:** Go modules + Makefile
- **Packaging:**
  - macOS: .dmg installer
  - Windows: .exe installer (NSIS/Wix)
  - Linux: .deb, .rpm, AppImage
- **Cross-compilation:** Supported via Go toolchain

---

## System Architecture

```
+-------------------------------------------------------------+
|                     Web UI (React)                          |
|  - Setup Wizard (OAuth flows)                               |
|  - Storage Dashboard (usage visualization)                  |
|  - File Browser (virtual filesystem view)                   |
|  - Settings (provider management, sync preferences)         |
+------------------------+------------------------------------+
                         | HTTP/WS (localhost:8080)
+------------------------v------------------------------------+
|              CloudUnify Service (Go)                        |
|                                                             |
|  +-----------------------------------------------------+   |
|  |              REST API Server                        |   |
|  |  - /api/providers (list, add, remove)               |   |
|  |  - /api/storage (usage stats)                       |   |
|  |  - /api/files (browse cloud files)                  |   |
|  |  - /api/sync (trigger operations)                   |   |
|  |  - /ws (WebSocket for real-time updates)            |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  |          FUSE Virtual Filesystem                    |   |
|  |  - Mount at ~/CloudUnify (configurable)             |   |
|  |  - Intercept file operations (read/write/delete)    |   |
|  |  - Stream files on-demand from cloud                |   |
|  |  - Handle local caching                             |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  |            Sync Engine                              |   |
|  |  - Upload Queue (priority-based)                    |   |
|  |  - Download Queue (on-demand)                       |   |
|  |  - Worker Pool (configurable concurrency)           |   |
|  |  - Retry Logic (exponential backoff)                |   |
|  |  - Progress Tracking                                |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  |         Cloud Provider Clients                      |   |
|  |  - GoogleDriveClient (OAuth2 + Drive API v3)        |   |
|  |  - OneDriveClient (OAuth2 + Graph API)              |   |
|  |  - iCloudClient (WebDAV/CloudKit)                   |   |
|  |  - Interface: Upload/Download/Delete/List/GetQuota  |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  |         Storage Allocation Strategy                 |   |
|  |  - Algorithm: Balance by usage percentage           |   |
|  |  - Fallback: Most available space                   |   |
|  |  - Constraint: One file = one provider (no split)   |   |
|  +-----------------------------------------------------+   |
|                                                             |
|  +-----------------------------------------------------+   |
|  |         Metadata Database (SQLite)                  |   |
|  |  - Files table (path, provider, cloud_id, size)     |   |
|  |  - Providers table (name, type, quota, used)        |   |
|  |  - SyncQueue table (pending operations)             |   |
|  |  - Cache table (local file cache tracking)          |   |
|  +-----------------------------------------------------+   |
+-------------------------------------------------------------+
```

---

## Database Schema

### Files Table
```sql
CREATE TABLE files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    virtual_path TEXT NOT NULL UNIQUE,        -- /Movies/video.mp4
    provider_id INTEGER NOT NULL,              -- FK to providers
    cloud_file_id TEXT NOT NULL,               -- Provider's file ID
    cloud_path TEXT,                           -- Path in provider's storage
    size_bytes INTEGER NOT NULL,
    checksum TEXT,                             -- SHA256 for integrity
    mime_type TEXT,
    status TEXT NOT NULL,                      -- 'synced', 'uploading', 'pending', 'error'
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES providers(id)
);

CREATE INDEX idx_files_virtual_path ON files(virtual_path);
CREATE INDEX idx_files_provider ON files(provider_id);
CREATE INDEX idx_files_status ON files(status);
```

### Providers Table
```sql
CREATE TABLE providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,                        -- 'My Google Drive'
    type TEXT NOT NULL,                        -- 'google_drive', 'onedrive', 'icloud'
    enabled BOOLEAN DEFAULT 1,
    quota_bytes INTEGER,                       -- Total storage
    used_bytes INTEGER DEFAULT 0,              -- Current usage
    access_token TEXT,                         -- Encrypted OAuth token
    refresh_token TEXT,                        -- Encrypted OAuth refresh token
    token_expiry DATETIME,
    config JSON,                               -- Provider-specific config
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Sync Queue Table
```sql
CREATE TABLE sync_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    operation TEXT NOT NULL,                   -- 'upload', 'download', 'delete'
    virtual_path TEXT NOT NULL,
    local_path TEXT,                           -- Temp file location for uploads
    provider_id INTEGER,
    priority INTEGER DEFAULT 0,                -- Higher = more urgent
    status TEXT NOT NULL,                      -- 'pending', 'processing', 'completed', 'failed'
    progress_percent INTEGER DEFAULT 0,
    error_message TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES providers(id)
);

CREATE INDEX idx_sync_queue_status ON sync_queue(status, priority DESC);
```

### Cache Table
```sql
CREATE TABLE cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_id INTEGER NOT NULL,
    local_path TEXT NOT NULL,                  -- Path in cache directory
    size_bytes INTEGER NOT NULL,
    last_accessed DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id)
);

CREATE INDEX idx_cache_last_accessed ON cache(last_accessed);
```

---

## Project Structure

```
cloud-storage-sync/
|-- cmd/
|   +-- cloudunify/
|       +-- main.go                    # Entry point, starts service
|-- internal/
|   |-- api/
|   |   |-- server.go                  # HTTP server setup
|   |   |-- handlers.go                # REST API handlers
|   |   +-- websocket.go               # WebSocket handler
|   |-- fuse/
|   |   |-- filesystem.go              # FUSE implementation
|   |   |-- operations.go              # File operations (read/write/etc)
|   |   +-- cache.go                   # Local file caching
|   |-- providers/
|   |   |-- interface.go               # CloudProvider interface
|   |   |-- google_drive.go            # Google Drive implementation
|   |   |-- onedrive.go                # OneDrive implementation
|   |   |-- icloud.go                  # iCloud implementation
|   |   +-- oauth.go                   # OAuth flow helpers
|   |-- sync/
|   |   |-- engine.go                  # Sync engine coordinator
|   |   |-- upload.go                  # Upload worker
|   |   |-- download.go                # Download worker
|   |   +-- queue.go                   # Queue management
|   |-- storage/
|   |   |-- allocator.go               # Storage allocation strategy
|   |   +-- tracker.go                 # Usage tracking
|   |-- database/
|   |   |-- db.go                      # Database connection
|   |   |-- models.go                  # Data models
|   |   +-- migrations.go              # Schema migrations
|   +-- config/
|       |-- config.go                  # Configuration management
|       +-- paths.go                   # Platform-specific paths
|-- web/
|   |-- src/
|   |   |-- components/
|   |   |   |-- SetupWizard.jsx        # Initial setup flow
|   |   |   |-- StorageDashboard.jsx   # Usage visualization
|   |   |   |-- FileBrowser.jsx        # File listing
|   |   |   |-- ProviderCard.jsx       # Provider status card
|   |   |   +-- SyncProgress.jsx       # Upload/download progress
|   |   |-- pages/
|   |   |   |-- Setup.jsx              # Setup page
|   |   |   |-- Dashboard.jsx          # Main dashboard
|   |   |   |-- Files.jsx              # File browser page
|   |   |   +-- Settings.jsx           # Settings page
|   |   |-- services/
|   |   |   |-- api.js                 # API client
|   |   |   +-- websocket.js           # WebSocket client
|   |   |-- App.jsx                    # Root component
|   |   +-- main.jsx                   # Entry point
|   |-- index.html
|   |-- package.json
|   +-- vite.config.js
|-- pkg/
|   +-- (reusable packages if needed)
|-- scripts/
|   |-- build.sh                       # Build script
|   |-- package-macos.sh               # macOS packaging
|   |-- package-windows.sh             # Windows packaging
|   +-- package-linux.sh               # Linux packaging
|-- docs/
|   |-- API.md                         # API documentation
|   |-- DEVELOPMENT.md                 # Development guide
|   +-- USER_GUIDE.md                  # User documentation
|-- go.mod
|-- go.sum
|-- Makefile
|-- README.md
|-- SPEC.md                            # This file
+-- .gitignore
```

---

## Core Interfaces

### CloudProvider Interface
```go
type CloudProvider interface {
    // Authentication
    Authenticate(ctx context.Context, config AuthConfig) error
    RefreshToken(ctx context.Context) error

    // File Operations
    Upload(ctx context.Context, localPath string, remotePath string) (FileMetadata, error)
    Download(ctx context.Context, fileID string, writer io.Writer) error
    Delete(ctx context.Context, fileID string) error

    // Metadata Operations
    GetFile(ctx context.Context, fileID string) (FileMetadata, error)
    ListFiles(ctx context.Context, path string) ([]FileMetadata, error)

    // Storage Information
    GetQuota(ctx context.Context) (QuotaInfo, error)

    // Stream support for large files
    UploadStream(ctx context.Context, reader io.Reader, remotePath string, size int64) (FileMetadata, error)
    DownloadStream(ctx context.Context, fileID string) (io.ReadCloser, error)
}

type FileMetadata struct {
    ID          string
    Name        string
    Path        string
    Size        int64
    MimeType    string
    Checksum    string
    ModTime     time.Time
}

type QuotaInfo struct {
    TotalBytes int64
    UsedBytes  int64
    FreeBytes  int64
}
```

### Storage Allocator Interface
```go
type StorageAllocator interface {
    // Choose best provider for a file
    ChooseProvider(ctx context.Context, fileSize int64) (*Provider, error)

    // Update provider usage stats
    UpdateUsage(ctx context.Context, providerID int64, delta int64) error

    // Get aggregated storage info
    GetTotalStorage(ctx context.Context) (QuotaInfo, error)
}
```

---

## Performance Specifications

### Target Performance Metrics

| Metric | Target | Critical? |
|--------|--------|-----------|
| Cold start time | < 200ms | Yes |
| Mount time | < 500ms | Yes |
| File listing (1000 files) | < 1s | Yes |
| Upload queue latency | < 100ms | No |
| Download start latency | < 500ms | Yes |
| Memory idle | < 50MB | No |
| Memory during transfer | < 200MB | No |
| CPU idle | < 1% | No |

### Concurrency Limits
- **Upload workers:** 3 concurrent (configurable)
- **Download workers:** 5 concurrent (configurable)
- **API rate limiting:** Respect provider limits
  - Google Drive: 1000 requests/100s/user
  - OneDrive: 5 requests/second
  - iCloud: TBD (WebDAV limits)

### Caching Strategy
- **Cache location:** `~/.cloudunify/cache/`
- **Cache size limit:** 10GB (configurable)
- **Eviction policy:** LRU (Least Recently Used)
- **Cache warming:** Pre-fetch frequently accessed files
- **Partial caching:** Support range requests for large video files

---

## File Operations Flow

### Upload Flow
```
1. User copies file to ~/CloudUnify/Movies/video.mp4
   v
2. FUSE intercepts write operation
   v
3. File written to temporary staging area (~/.cloudunify/staging/)
   v
4. Create entry in sync_queue table (operation='upload')
   v
5. Sync engine picks up job
   v
6. StorageAllocator chooses provider (e.g., Google Drive has most space)
   v
7. Upload worker starts streaming upload
   v
8. Progress updates sent via WebSocket to UI
   v
9. On success:
   - Create entry in files table
   - Update provider usage in providers table
   - Mark sync_queue entry as completed
   - Delete staging file (or move to cache)
   v
10. FUSE now shows file as available in virtual filesystem
```

### Download Flow
```
1. User opens ~/CloudUnify/Movies/video.mp4
   v
2. FUSE intercepts read operation
   v
3. Check cache table for local copy
   v
4. If cached:
   - Update last_accessed timestamp
   - Return file from cache
   v
5. If not cached:
   - Query files table for cloud location
   - Get provider and cloud_file_id
   - Create download job in sync_queue
   v
6. Download worker streams file from provider
   v
7. Stream data directly to application (no wait for full download)
   v
8. Simultaneously write to cache
   v
9. Progress updates sent via WebSocket to UI
   v
10. On completion, update cache table
```

### Delete Flow
```
1. User deletes ~/CloudUnify/Movies/video.mp4
   v
2. FUSE intercepts unlink operation
   v
3. Look up file in files table
   v
4. Create delete job in sync_queue
   v
5. Delete from cloud provider
   v
6. On success:
   - Delete from files table
   - Update provider usage
   - Delete from cache if present
   - Mark sync_queue entry as completed
```

---

## API Endpoints

### Provider Management
```
GET    /api/providers              # List all providers
POST   /api/providers              # Add new provider (initiates OAuth)
GET    /api/providers/:id          # Get provider details
DELETE /api/providers/:id          # Remove provider
POST   /api/providers/:id/refresh  # Refresh OAuth token
GET    /api/providers/:id/quota    # Get storage quota
```

### Storage Information
```
GET    /api/storage                # Aggregated storage stats
GET    /api/storage/usage          # Usage breakdown by provider
```

### File Management
```
GET    /api/files                  # List files (supports pagination, filtering)
GET    /api/files/*path            # Get file metadata
DELETE /api/files/*path            # Delete file
POST   /api/files/search           # Search files
```

### Sync Operations
```
GET    /api/sync/queue             # View sync queue
GET    /api/sync/status            # Current sync status
POST   /api/sync/pause             # Pause syncing
POST   /api/sync/resume            # Resume syncing
DELETE /api/sync/queue/:id         # Cancel queued operation
```

### System
```
GET    /api/health                 # Health check
GET    /api/version                # Version info
POST   /api/shutdown               # Graceful shutdown
```

### WebSocket
```
WS     /ws                         # Real-time updates
   ->  Events: sync_progress, file_added, file_deleted, provider_updated, error
```

---

## OAuth Flow

### Google Drive
```
1. User clicks "Add Google Drive" in UI
2. Backend generates OAuth URL with scopes:
   - https://www.googleapis.com/auth/drive.file
   - https://www.googleapis.com/auth/drive.metadata.readonly
3. Frontend opens URL in browser
4. User grants permission
5. Google redirects to http://localhost:8080/oauth/callback?code=...
6. Backend exchanges code for access_token + refresh_token
7. Store tokens (encrypted) in providers table
8. Test connection and get quota
9. Provider added successfully
```

### OneDrive (similar flow)
```
Scopes:
- Files.ReadWrite.All
- User.Read
```

### iCloud
```
Option 1: WebDAV (username + app-specific password)
Option 2: CloudKit (requires Apple Developer account)
```

---

## Security Considerations

### Token Storage
- OAuth tokens encrypted at rest using AES-256
- Encryption key derived from system keychain (macOS) / Credential Manager (Windows) / Secret Service (Linux)
- Never log tokens

### File Integrity
- SHA256 checksum verification on upload/download
- Detect corruption and retry

### Permissions
- Virtual filesystem respects user file permissions
- No root/admin required

### HTTPS
- All cloud provider API calls over HTTPS
- Certificate pinning for critical operations

---

## Development Phases

### Phase 1: Foundation (Weeks 1-2)
- [ ] Set up Go project structure
- [ ] Initialize SQLite database with migrations
- [ ] Implement config management
- [ ] Build basic HTTP server
- [ ] Create provider interface
- [ ] Implement Google Drive provider (OAuth + basic operations)

### Phase 2: Filesystem (Weeks 3-4)
- [ ] Implement FUSE filesystem skeleton
- [ ] Basic file operations (read, write, delete)
- [ ] Integrate with sync engine
- [ ] Test mounting on macOS

### Phase 3: Sync Engine (Weeks 5-6)
- [ ] Build upload queue and workers
- [ ] Build download queue and workers
- [ ] Implement retry logic
- [ ] Add progress tracking
- [ ] Storage allocator implementation

### Phase 4: Additional Providers (Week 7)
- [ ] Implement OneDrive provider
- [ ] Implement iCloud provider (WebDAV)
- [ ] Test multi-provider scenarios

### Phase 5: Web UI (Weeks 8-9)
- [ ] Setup Vite + React project
- [ ] Build setup wizard
- [ ] Create storage dashboard
- [ ] Implement file browser
- [ ] WebSocket integration for real-time updates

### Phase 6: Caching & Optimization (Week 10)
- [ ] Implement local file cache
- [ ] LRU eviction policy
- [ ] Partial file caching for videos
- [ ] Performance testing and optimization

### Phase 7: Cross-platform (Weeks 11-12)
- [ ] Test on Windows with WinFsp
- [ ] Test on Linux with FUSE
- [ ] Build installers for all platforms
- [ ] Platform-specific bug fixes

### Phase 8: Polish & Release (Weeks 13-14)
- [ ] Error handling improvements
- [ ] User documentation
- [ ] Edge case testing
- [ ] Beta release
- [ ] Gather feedback and iterate

---

## Testing Strategy

### Unit Tests
- All provider implementations
- Storage allocator logic
- Database operations
- API handlers

### Integration Tests
- End-to-end OAuth flows
- Upload/download with real providers (using test accounts)
- FUSE operations
- Multi-provider scenarios

### Performance Tests
- Large file transfers (10GB+)
- Concurrent operations (100+ files)
- Memory profiling
- CPU profiling

### Manual Testing
- User acceptance testing
- Cross-platform compatibility
- Real-world usage scenarios

---

## Monitoring & Logging

### Logging Levels
- **DEBUG:** Detailed operation logs (disabled in production)
- **INFO:** Normal operations (uploads, downloads, mounts)
- **WARN:** Recoverable errors, retries
- **ERROR:** Failures requiring attention

### Metrics to Track
- Upload/download throughput (MB/s)
- API error rates by provider
- Queue lengths
- Cache hit rate
- Active file operations

### Log Locations
- macOS: `~/Library/Logs/CloudUnify/`
- Windows: `%APPDATA%\CloudUnify\Logs\`
- Linux: `~/.local/share/cloudunify/logs/`

---

## Configuration

### Config File Location
- macOS: `~/Library/Application Support/CloudUnify/config.json`
- Windows: `%APPDATA%\CloudUnify\config.json`
- Linux: `~/.config/cloudunify/config.json`

### Sample Configuration
```json
{
  "mount_path": "~/CloudUnify",
  "cache": {
    "enabled": true,
    "max_size_gb": 10,
    "path": "~/.cloudunify/cache"
  },
  "sync": {
    "upload_workers": 3,
    "download_workers": 5,
    "auto_sync": true
  },
  "allocation_strategy": "balanced_usage",
  "api": {
    "port": 8080,
    "host": "localhost"
  },
  "logging": {
    "level": "info",
    "file": "~/.cloudunify/logs/cloudunify.log"
  }
}
```

---

## Known Limitations

### v1.0 Scope
- **No file versioning** (use provider's native versioning)
- **No file sharing** (manage directly in provider's UI)
- **No offline mode** (files must download before access)
- **No encryption** (files stored as-is in cloud)
- **No deduplication** (same file uploaded twice = 2x storage)
- **No mobile apps** (desktop only)

### Provider-Specific
- **iCloud:** Limited API access, may require WebDAV (slower)
- **Rate limits:** May throttle during bulk operations
- **File size limits:** Some providers have per-file limits

---

## Future Enhancements (v2.0+)

### High Priority
- [ ] Client-side encryption (encrypt before upload)
- [ ] File versioning and history
- [ ] Selective sync (choose which folders to mount)
- [ ] Bandwidth throttling controls
- [ ] Folder watch for bidirectional sync

### Medium Priority
- [ ] File deduplication (content-addressable storage)
- [ ] Compression before upload
- [ ] Redundancy mode (store critical files on multiple providers)
- [ ] Team sharing features
- [ ] Mobile companion app

### Low Priority
- [ ] Plugin system for additional providers
- [ ] Advanced caching strategies (predictive prefetch)
- [ ] Statistics and analytics
- [ ] Backup and restore functionality

---

## Success Metrics

### Technical Metrics
- Zero data loss
- < 0.1% failed operations
- 99% uptime of background service
- < 5s cold start on all platforms

### User Metrics
- Successful setup in < 5 minutes
- Storage correctly aggregated
- Files accessible within 1s of open
- Transparent operation (user doesn't think about which cloud)

---

## Dependencies & Prerequisites

### Development Environment
- Go 1.21 or higher
- Node.js 18+ and npm/yarn (for web UI)
- macFUSE (macOS), WinFsp (Windows), or FUSE (Linux)
- SQLite 3.35+
- Git

### Cloud Accounts Required
- Google account with Drive API enabled
- Microsoft account with OneDrive
- Apple ID with iCloud+

### API Credentials
- Google Cloud Console: Create OAuth 2.0 credentials
- Azure Portal: Register app for OneDrive access
- Apple Developer (optional): For CloudKit access

---

## Build Instructions

### Development Build
```bash
# Backend
make build-dev

# Frontend
cd web
npm install
npm run dev

# Run service
./bin/cloudunify
```

### Production Build
```bash
# Build for current platform
make build

# Cross-compile for all platforms
make build-all

# Create installers
make package-macos
make package-windows
make package-linux
```

### Environment Variables
```
CLOUDUNIFY_CONFIG_PATH   # Custom config location
CLOUDUNIFY_LOG_LEVEL     # Override log level
CLOUDUNIFY_DEV_MODE      # Enable development features
```

---

## Contributing Guidelines

### Code Style
- Follow standard Go conventions (gofmt, golint)
- Write tests for new features
- Document exported functions
- Keep functions small and focused

### Commit Messages
```
feat: add OneDrive provider support
fix: resolve upload retry logic bug
docs: update API documentation
perf: optimize file listing query
```

### Pull Request Process
1. Fork repository
2. Create feature branch
3. Write tests
4. Ensure all tests pass
5. Submit PR with description

---

## License

TBD (MIT recommended for open source)

---

## References

### Similar Projects
- **rclone:** https://rclone.org/ (CLI tool, excellent reference)
- **Duplicati:** Backup solution with multiple cloud support
- **ExpanDrive:** Commercial cloud mounting solution

### Documentation Links
- cgofuse: https://github.com/winfsp/cgofuse
- Google Drive API: https://developers.google.com/drive
- Microsoft Graph API: https://docs.microsoft.com/graph
- macFUSE: https://osxfuse.github.io/

---

## Contact & Support

- **Issues:** GitHub Issues
- **Discussions:** GitHub Discussions
- **Email:** TBD

---

**Last Updated:** 2026-01-09
**Version:** 1.0-SPEC
**Status:** Phase 0 - Core Hardening
