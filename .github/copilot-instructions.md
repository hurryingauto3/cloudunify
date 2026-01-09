# CloudUnify - GitHub Copilot Instructions

## Project Context

CloudUnify is a unified cloud storage system written in Go that mounts multiple cloud providers (Google Drive, OneDrive, iCloud) as a single virtual filesystem at `~/CloudUnify`. Files copied to this folder are automatically uploaded to the cloud.

## Tech Stack

- **Backend:** Go 1.21+, cgofuse, SQLite, gorilla/mux
- **Frontend:** React + Vite
- **Filesystem:** macFUSE (macOS), FUSE (Linux), WinFsp (Windows)
- **Cloud APIs:** Google Drive API v3, Microsoft Graph API

## Project Structure

```
cmd/cloudunify/main.go          # Entry point
internal/
  api/handlers.go               # REST API handlers
  api/server.go                 # HTTP server setup
  api/websocket.go              # WebSocket for real-time updates
  fuse/filesystem.go            # FUSE filesystem implementation
  providers/interface.go        # CloudProvider interface
  providers/google_drive.go     # Google Drive implementation
  providers/onedrive.go         # OneDrive implementation (stub)
  providers/icloud.go           # iCloud implementation (stub)
  sync/engine.go                # Background sync workers
  sync/queue.go                 # Priority queue
  database/db.go                # SQLite operations
  database/models.go            # Data types
  config/config.go              # Configuration
web/                            # React frontend
```

## Code Style

### Go Imports
```go
import (
    // stdlib first
    "context"
    "fmt"
    
    // third-party
    "github.com/winfsp/cgofuse/fuse"
    
    // internal packages
    "cloudunify/internal/database"
)
```

### Error Handling
```go
// Always wrap errors with context
if err != nil {
    return fmt.Errorf("failed to upload file: %w", err)
}
```

### Logging
```go
// Use log.Printf with component prefix
log.Printf("FUSE Create: %s", path)
log.Printf("Upload complete: %s", virtualPath)
```

### FUSE Operations
```go
// Return negative fuse error codes
func (fs *CloudUnifyFS) Read(...) int {
    if err != nil {
        return -fuse.EIO
    }
    return bytesRead
}
```

## Key Patterns

### File Upload Flow
1. FUSE `Create` → Create staging file in `~/Library/Caches/CloudUnify/staging/`
2. FUSE `Write` → Write to staging file
3. FUSE `Release` → Queue upload to sync engine
4. Sync worker → Upload to cloud provider
5. Update database with cloud file ID

### Provider Interface
```go
type CloudProvider interface {
    Authenticate(ctx context.Context, config AuthConfig) error
    Upload(ctx context.Context, localPath, remotePath string) (*FileMetadata, error)
    UploadStream(ctx context.Context, reader io.Reader, path string, size int64) (*FileMetadata, error)
    Download(ctx context.Context, fileID string, writer io.Writer) error
    GetQuota(ctx context.Context) (*QuotaInfo, error)
}
```

### Database Tables
- `providers` - Cloud provider configs and OAuth tokens
- `files` - Virtual path → cloud file mapping
- `sync_queue` - Pending upload/download operations
- `cache` - Local file cache tracking

## Common Tasks

### Add API Endpoint
1. Add handler method to `internal/api/handlers.go`
2. Register route in `internal/api/server.go`

### Add FUSE Operation
1. Implement method in `internal/fuse/filesystem.go`
2. Follow cgofuse interface signature
3. Return negative error codes

### Add Provider
1. Create `internal/providers/provider_name.go`
2. Implement `CloudProvider` interface
3. Register in provider manager

## Testing

```bash
# Build
go build -o bin/cloudunify ./cmd/cloudunify

# Run
./bin/cloudunify

# Test file copy
cp /path/to/file ~/CloudUnify/

# Check API
curl http://localhost:8080/api/files | jq
curl http://localhost:8080/api/sync/queue | jq
```

## Important Notes

- Files are staged locally before upload (async upload)
- FUSE operations must be fast (<50ms overhead)
- Check staging directory before database in Getattr
- Implement Flush and Setattr for macOS Finder compatibility
- Use resumable upload for files >5MB
