# CloudUnify - AI Assistant Instructions

This document provides instructions for AI assistants working on the CloudUnify project.

---

## Project Overview

**CloudUnify** is a unified cloud storage system that mounts multiple cloud providers as a single virtual filesystem at `~/CloudUnify`.

| Aspect | Details |
|--------|---------|
| Language | Go 1.21+ |
| Filesystem | cgofuse + macFUSE |
| Database | SQLite |
| Frontend | React + Vite |
| API | REST + WebSocket on :8080 |

---

## Current Implementation Status

### ✅ Working Features
- FUSE mount at `~/CloudUnify`
- Drag-and-drop file upload (Finder + terminal)
- Google Drive OAuth 2.0 authentication
- Multipart upload (≤5MB) and resumable upload (>5MB)
- Background sync engine with 3 upload workers
- File staging at `~/Library/Caches/CloudUnify/staging`
- Real-time WebSocket progress updates
- React dashboard

### 🚧 Not Yet Implemented
- OneDrive integration
- iCloud integration
- Download on file read
- Smart file distribution across providers
- File conflict resolution

---

## Key Architecture

```
~/CloudUnify (FUSE Mount)
       │
       ▼
┌──────────────────┐
│  CloudUnify      │
│  Server          │
│                  │
│  ├─ FUSE FS      │ ─→ Intercepts file ops
│  ├─ Sync Engine  │ ─→ Background upload/download
│  ├─ REST API     │ ─→ :8080
│  └─ Providers    │ ─→ Google Drive, OneDrive, iCloud
└──────────────────┘
       │
       ▼
   Cloud Storage
```

---

## Critical Files

| File | Purpose |
|------|---------|
| `internal/fuse/filesystem.go` | FUSE operations (Create, Read, Write, Open, etc.) |
| `internal/providers/google_drive.go` | Google Drive API integration |
| `internal/sync/engine.go` | Background upload/download workers |
| `internal/sync/queue.go` | Priority queue for sync operations |
| `internal/api/handlers.go` | REST API endpoints |
| `internal/database/db.go` | SQLite operations |

---

## Common Development Tasks

### Adding a FUSE Operation
1. Implement the method in `internal/fuse/filesystem.go`
2. Follow cgofuse interface signatures
3. Return negative fuse error codes (e.g., `-fuse.EIO`)
4. Add logging with `log.Printf("FUSE OperationName: %s", path)`

### Adding an API Endpoint
1. Add handler in `internal/api/handlers.go`
2. Register route in `internal/api/server.go`
3. Follow pattern: `h.router.HandleFunc("/api/...", h.handlerName).Methods("GET")`

### Adding Provider Support
1. Implement `CloudProvider` interface from `internal/providers/interface.go`
2. Key methods: `Authenticate`, `Upload`, `UploadStream`, `Download`, `GetQuota`
3. Register in provider manager

---

## Debugging Tips

### Check Server Logs
```bash
cat /tmp/cloudunify.log | tail -50
cat /tmp/cloudunify.log | grep -E "error|Error|ERROR"
```

### Check Sync Queue
```bash
sqlite3 ~/Library/Application\ Support/CloudUnify/cloudunify.db \
  "SELECT id, operation, virtual_path, status FROM sync_queue ORDER BY id DESC LIMIT 10;"
```

### Check Files Table
```bash
sqlite3 ~/Library/Application\ Support/CloudUnify/cloudunify.db \
  "SELECT virtual_path, size_bytes, status FROM files ORDER BY id DESC LIMIT 10;"
```

### Unmount FUSE
```bash
umount ~/CloudUnify
# or
diskutil unmount force ~/CloudUnify
```

---

## Code Style

### Go Conventions
```go
// Package imports: stdlib, third-party, internal
import (
    "context"
    "fmt"
    
    "github.com/winfsp/cgofuse/fuse"
    
    "cloudunify/internal/database"
)

// Error handling: wrap with context
if err != nil {
    return fmt.Errorf("failed to upload file: %w", err)
}

// Logging: use log.Printf with component prefix
log.Printf("FUSE Create: %s", path)
log.Printf("Upload complete: %s", virtualPath)
```

### File Naming
- Go files: `snake_case.go`
- React components: `PascalCase.jsx`
- Test files: `*_test.go`

---

## Testing Commands

```bash
# Build
go build -o bin/cloudunify ./cmd/cloudunify

# Test file copy
cp /tmp/testfile.txt ~/CloudUnify/

# Test via API
curl -X POST http://localhost:8080/api/files/upload \
  -F "file=@/tmp/test.txt" \
  -F "path=/test.txt"

# Check providers
curl http://localhost:8080/api/providers | jq

# Check files
curl http://localhost:8080/api/files | jq
```

---

## Important Considerations

### Performance
- FUSE operations should be fast (<50ms overhead)
- Use staging directory for writes, async upload
- Never buffer entire large files in memory

### Data Integrity
- Always verify upload success before marking complete
- Update database records after successful cloud operations
- Handle unique constraint errors by updating existing records

### macOS Compatibility
- Implement `Setattr`, `Flush`, `Getxattr`, `Setxattr` for Finder
- Check staging directory before database in `Getattr`
- Handle files with extended attributes

---

## Quick Reference

| Setting | Value |
|---------|-------|
| Mount Point | `~/CloudUnify` |
| Config Dir | `~/Library/Application Support/CloudUnify/` |
| Cache Dir | `~/Library/Caches/CloudUnify/` |
| API Port | 8080 |
| Web UI Port | 5173 |
| Upload Workers | 3 |
| Download Workers | 5 |
