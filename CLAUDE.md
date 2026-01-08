# CloudUnify - AI Assistant Instructions

This document contains comprehensive instructions for AI assistants (like Claude) working on the CloudUnify project. Follow these guidelines when helping with development, debugging, or feature implementation.

---

## Project Context

**Project Name:** CloudUnify
**Purpose:** Unified cloud storage system that mounts multiple cloud providers (iCloud, Google Drive, OneDrive) as a single virtual filesystem
**Tech Stack:** Go + cgofuse + SQLite + React
**Target Platforms:** macOS, Windows, Linux
**Primary Reference:** See `SPEC.md` for complete technical specifications

---

## Core Principles

### 1. **Always Refer to SPEC.md First**
- Before implementing any feature, read the relevant section in SPEC.md
- Follow the defined architecture, database schema, and API contracts
- Don't deviate from the spec without explicit user approval
- If the spec is unclear, ask for clarification before proceeding

### 2. **Performance is Critical**
- This is a filesystem operation - performance matters
- Keep FUSE operations fast (target: read/write < 50ms overhead)
- Stream large files, never buffer entire files in memory
- Use goroutines for concurrent operations but limit concurrency
- Profile code if performance issues arise

### 3. **No Data Loss**
- Data integrity is paramount
- Always verify checksums after upload/download
- Implement proper error handling and retry logic
- Never delete from cloud without database confirmation
- Test edge cases thoroughly

### 4. **Security First**
- Encrypt OAuth tokens at rest (AES-256)
- Never log sensitive data (tokens, file contents)
- Use HTTPS for all cloud API calls
- Validate all user inputs
- Follow OAuth 2.0 best practices

---

## Project Structure Rules

### Directory Organization
```
cmd/          - Entry points (main.go)
internal/     - Private application code (not importable)
pkg/          - Public libraries (reusable, if needed)
web/          - Frontend React application
scripts/      - Build and packaging scripts
docs/         - Documentation
```

### Naming Conventions
- **Files:** snake_case (e.g., `google_drive.go`)
- **Packages:** lowercase, single word (e.g., `providers`, `sync`)
- **Types:** PascalCase (e.g., `CloudProvider`, `FileMetadata`)
- **Functions/Methods:** camelCase (e.g., `uploadFile`, `getQuota`)
- **Constants:** PascalCase or UPPER_SNAKE_CASE depending on scope

### Import Organization
```go
import (
    // Standard library
    "context"
    "fmt"

    // Third-party packages
    "github.com/winfsp/cgofuse/fuse"

    // Internal packages
    "cloudunify/internal/database"
    "cloudunify/internal/providers"
)
```

---

## Implementation Guidelines

### When Adding a New Feature

1. **Understand the Requirement**
   - Read the relevant SPEC.md section
   - Identify affected components (database, API, FUSE, providers)
   - Ask clarifying questions if needed

2. **Design First**
   - Sketch out the data flow
   - Identify new database tables/columns needed
   - Define API endpoints if applicable
   - Consider error cases

3. **Implement Incrementally**
   - Start with database schema changes (add migration)
   - Implement core logic
   - Add API endpoints
   - Update FUSE operations if needed
   - Add frontend UI last

4. **Test Thoroughly**
   - Write unit tests for new functions
   - Test with real cloud providers (use test accounts)
   - Test error scenarios (network failure, auth errors, etc.)
   - Verify no data loss or corruption

5. **Document**
   - Add godoc comments to exported functions
   - Update API.md if endpoints changed
   - Update USER_GUIDE.md if user-facing

### Code Quality Standards

#### Error Handling
```go
// ✅ GOOD: Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to upload file to %s: %w", provider.Name, err)
}

// ❌ BAD: Swallow errors or lose context
if err != nil {
    log.Println(err)
    return nil
}
```

#### Context Usage
```go
// ✅ GOOD: Pass context, respect cancellation
func (p *GoogleDriveProvider) Upload(ctx context.Context, file string) error {
    req := p.client.Files.Create(metadata)
    req.Context(ctx) // Respect cancellation

    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
        return req.Do()
    }
}

// ❌ BAD: Ignore context
func (p *GoogleDriveProvider) Upload(file string) error {
    // No context, can't cancel long operations
}
```

#### Resource Management
```go
// ✅ GOOD: Always close resources
func downloadFile(ctx context.Context, url string) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close() // Always close

    // Process response...
}
```

#### Database Transactions
```go
// ✅ GOOD: Use transactions for multi-step operations
tx, err := db.Begin()
if err != nil {
    return err
}
defer tx.Rollback() // Safe to call even after commit

// Multiple operations...
if err := insertFile(tx, file); err != nil {
    return err // Rollback happens automatically
}
if err := updateProviderUsage(tx, provider, size); err != nil {
    return err
}

return tx.Commit()
```

---

## Provider Implementation

### When Adding a New Cloud Provider

Follow this checklist:

1. **Create provider file:** `internal/providers/{provider_name}.go`

2. **Implement CloudProvider interface:**
```go
type GoogleDriveProvider struct {
    client *drive.Service
    config ProviderConfig
}

// Must implement ALL interface methods:
// - Authenticate()
// - RefreshToken()
// - Upload() / UploadStream()
// - Download() / DownloadStream()
// - Delete()
// - GetFile()
// - ListFiles()
// - GetQuota()
```

3. **Handle OAuth flow:**
   - Store tokens encrypted in database
   - Implement token refresh logic
   - Handle revoked tokens gracefully

4. **Implement streaming:**
   - Use `io.Reader` and `io.Writer` interfaces
   - Never load entire file into memory
   - Support resumable uploads if provider allows

5. **Error mapping:**
   - Convert provider-specific errors to standard errors
   - Handle rate limiting (implement exponential backoff)
   - Detect quota exceeded errors

6. **Add tests:**
   - Unit tests with mocked API
   - Integration tests with real API (test account)

### Provider-Specific Notes

#### Google Drive
- Use Drive API v3
- Request minimal scopes: `drive.file` and `drive.metadata.readonly`
- Handle "My Drive" vs "Shared Drives" appropriately
- Respect `quotaBytesUsed` and `quotaBytesTotal`

#### OneDrive
- Use Microsoft Graph API
- Handle personal OneDrive vs OneDrive for Business
- Use delta queries for efficient file listing
- Support resumable uploads for files > 4MB

#### iCloud
- WebDAV is the most accessible option (no Apple Developer account needed)
- Base URL: `https://caldav.icloud.com/` or `https://contacts.icloud.com/`
- Requires app-specific password (not main iCloud password)
- CloudKit is more powerful but requires Apple Developer Program membership

---

## Database Guidelines

### Schema Changes

**Always use migrations:**
```go
// internal/database/migrations.go

var migrations = []Migration{
    {
        Version: 1,
        Up: `CREATE TABLE files (...)`,
        Down: `DROP TABLE files`,
    },
    {
        Version: 2,
        Up: `ALTER TABLE files ADD COLUMN checksum TEXT`,
        Down: `ALTER TABLE files DROP COLUMN checksum`,
    },
}
```

### Query Patterns

```go
// ✅ GOOD: Prepared statements, handle errors
func getFile(db *sql.DB, path string) (*File, error) {
    var f File
    err := db.QueryRow(
        "SELECT id, virtual_path, size_bytes FROM files WHERE virtual_path = ?",
        path,
    ).Scan(&f.ID, &f.VirtualPath, &f.Size)

    if err == sql.ErrNoRows {
        return nil, ErrFileNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("query failed: %w", err)
    }
    return &f, nil
}

// ❌ BAD: String concatenation (SQL injection risk)
func getFile(db *sql.DB, path string) (*File, error) {
    query := "SELECT * FROM files WHERE virtual_path = '" + path + "'"
    // NEVER DO THIS!
}
```

### Indexes

Add indexes for:
- Foreign keys
- Frequently queried columns (status, virtual_path)
- Sort columns (created_at, size_bytes)

```sql
CREATE INDEX idx_files_status ON files(status);
CREATE INDEX idx_files_virtual_path ON files(virtual_path);
CREATE INDEX idx_sync_queue_status_priority ON sync_queue(status, priority DESC);
```

---

## FUSE Implementation

### Key Principles

1. **Keep it Simple:**
   - Implement only necessary operations (read, write, readdir, etc.)
   - Return appropriate error codes (ENOENT, EACCES, etc.)

2. **Non-blocking:**
   - FUSE operations should return quickly
   - Offload heavy work (uploads/downloads) to background workers
   - Use caching aggressively

3. **Stateless:**
   - Don't rely on previous calls
   - Each operation should be self-contained

### Common Operations

```go
func (fs *CloudUnifyFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) int {
    // Return file attributes (size, mode, times)
    // Check database for file metadata
    // Return -fuse.ENOENT if not found
}

func (fs *CloudUnifyFS) Open(path string, flags int) (int, uint64) {
    // Called when file is opened
    // Start download if not cached
    // Return file handle
}

func (fs *CloudUnifyFS) Read(path string, buff []byte, ofst int64, fh uint64) int {
    // Read file contents at offset
    // Stream from cache or cloud
    // Support partial reads (important for video seeking)
}

func (fs *CloudUnifyFS) Write(path string, buff []byte, ofst int64, fh uint64) int {
    // Write to staging area
    // Queue for upload
    // Return bytes written
}

func (fs *CloudUnifyFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) int {
    // List directory contents
    // Query database for files under path
    // Call fill() for each entry
}
```

### Platform Differences

```go
// Use build tags for platform-specific code

// +build darwin

package fuse

// macOS-specific FUSE options
var platformOptions = []string{
    "-o", "volname=CloudUnify",
    "-o", "iosize=65536",
}
```

```go
// +build windows

package fuse

// Windows-specific WinFsp options
var platformOptions = []string{
    "--VolumePrefix=\\cloudunify",
}
```

---

## API Development

### Endpoint Design

Follow RESTful conventions:
- `GET /api/resource` - List
- `GET /api/resource/:id` - Get one
- `POST /api/resource` - Create
- `PUT /api/resource/:id` - Update
- `DELETE /api/resource/:id` - Delete

### Handler Structure

```go
func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
    // 1. Parse and validate input
    path := r.URL.Query().Get("path")
    if path == "" {
        respondError(w, http.StatusBadRequest, "path required")
        return
    }

    // 2. Call business logic
    file, err := s.fileService.GetFile(r.Context(), path)
    if err != nil {
        if errors.Is(err, ErrFileNotFound) {
            respondError(w, http.StatusNotFound, "file not found")
        } else {
            respondError(w, http.StatusInternalServerError, "internal error")
        }
        return
    }

    // 3. Return response
    respondJSON(w, http.StatusOK, file)
}
```

### Error Responses

Standard format:
```json
{
  "error": {
    "code": "FILE_NOT_FOUND",
    "message": "File not found at path /Movies/video.mp4",
    "details": {}
  }
}
```

### WebSocket Events

```go
type WSEvent struct {
    Type    string      `json:"type"`    // "sync_progress", "file_added", etc.
    Payload interface{} `json:"payload"`
    Time    time.Time   `json:"time"`
}

// Example: Send progress update
s.broadcast(WSEvent{
    Type: "sync_progress",
    Payload: map[string]interface{}{
        "file_id": fileID,
        "percent": 45,
        "bytes_transferred": 1024000,
    },
    Time: time.Now(),
})
```

---

## Sync Engine

### Queue Processing

```go
// Worker pool pattern
func (e *SyncEngine) Start(ctx context.Context, workers int) {
    for i := 0; i < workers; i++ {
        go e.worker(ctx, i)
    }
}

func (e *SyncEngine) worker(ctx context.Context, id int) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            job, err := e.dequeueJob(ctx)
            if err != nil {
                time.Sleep(time.Second)
                continue
            }

            e.processJob(ctx, job)
        }
    }
}
```

### Retry Logic

```go
func (e *SyncEngine) processJob(ctx context.Context, job *SyncJob) error {
    maxRetries := 3
    backoff := time.Second

    for attempt := 0; attempt <= maxRetries; attempt++ {
        err := e.executeJob(ctx, job)
        if err == nil {
            return nil // Success
        }

        if !isRetriable(err) {
            return err // Permanent failure
        }

        if attempt < maxRetries {
            time.Sleep(backoff)
            backoff *= 2 // Exponential backoff
        }
    }

    return fmt.Errorf("max retries exceeded")
}

func isRetriable(err error) bool {
    // Network errors, rate limits, temporary failures
    // NOT: authentication errors, file not found, quota exceeded
}
```

---

## Testing Strategies

### Unit Tests

```go
func TestStorageAllocator_ChooseProvider(t *testing.T) {
    // Setup
    allocator := NewStorageAllocator()
    providers := []*Provider{
        {ID: 1, Type: "google_drive", UsedBytes: 500 * GB, QuotaBytes: 2 * TB},
        {ID: 2, Type: "onedrive", UsedBytes: 100 * GB, QuotaBytes: 1 * TB},
    }

    // Execute
    chosen, err := allocator.ChooseProvider(context.Background(), providers, 10*GB)

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if chosen.ID != 2 {
        t.Errorf("expected OneDrive (ID=2), got ID=%d", chosen.ID)
    }
}
```

### Integration Tests

```go
// Use test accounts with small quotas
func TestGoogleDriveProvider_Upload_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test")
    }

    // Setup real provider
    provider := setupTestProvider(t)

    // Upload test file
    file := createTempFile(t, "test.txt", "hello world")
    defer os.Remove(file)

    metadata, err := provider.Upload(context.Background(), file, "/test.txt")
    if err != nil {
        t.Fatalf("upload failed: %v", err)
    }

    // Verify
    if metadata.Size != 11 {
        t.Errorf("expected size 11, got %d", metadata.Size)
    }

    // Cleanup
    provider.Delete(context.Background(), metadata.ID)
}
```

### Performance Tests

```go
func BenchmarkFUSE_Read(b *testing.B) {
    fs := setupTestFS(b)
    path := "/test.txt"
    buff := make([]byte, 4096)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        fs.Read(path, buff, 0, 0)
    }
}
```

---

## Common Tasks

### Adding a New API Endpoint

1. Define handler in `internal/api/handlers.go`
2. Register route in `internal/api/server.go`
3. Add to `docs/API.md`
4. Write tests
5. Update frontend API client if needed

### Adding a Database Table

1. Create migration in `internal/database/migrations.go`
2. Add model struct in `internal/database/models.go`
3. Add CRUD functions
4. Add indexes
5. Update SPEC.md schema section

### Debugging Issues

**FUSE not mounting:**
```bash
# Check if FUSE is installed
which macfuse  # macOS

# Check mount points
mount | grep cloudunify

# Enable debug logging
export CLOUDUNIFY_LOG_LEVEL=debug
./cloudunify
```

**OAuth not working:**
- Verify redirect URI matches exactly in cloud console
- Check token expiry
- Verify scopes are correct
- Test with OAuth playground first

**Upload/download failures:**
- Check network connectivity
- Verify API quotas not exceeded
- Check file size limits
- Review error logs for specific error codes

---

## Performance Optimization

### Profiling

```go
import _ "net/http/pprof"

// Add to main.go
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Then use:
```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine dump
curl http://localhost:6060/debug/pprof/goroutine?debug=1
```

### Optimization Checklist

- [ ] Use connection pooling for HTTP clients
- [ ] Implement request coalescing (deduplicate concurrent requests)
- [ ] Cache frequently accessed metadata
- [ ] Use streaming for large files (no buffering)
- [ ] Limit concurrent uploads/downloads
- [ ] Use appropriate buffer sizes (64KB for FUSE)
- [ ] Profile before optimizing (measure first!)

---

## Security Checklist

- [ ] OAuth tokens encrypted at rest
- [ ] Never log tokens or credentials
- [ ] Use HTTPS for all external requests
- [ ] Validate all user inputs
- [ ] Use prepared statements for SQL
- [ ] Implement rate limiting on API
- [ ] Set appropriate file permissions
- [ ] Clear sensitive data from memory after use
- [ ] Handle token revocation gracefully
- [ ] Use secure random for encryption keys

---

## Git Workflow

### Branch Naming
- `feature/provider-onedrive`
- `fix/upload-retry-logic`
- `docs/api-documentation`

### Commit Messages
```
feat: add OneDrive provider implementation
fix: resolve race condition in sync queue
docs: update API documentation for file endpoints
perf: optimize file listing query with index
test: add integration tests for Google Drive
```

### Before Committing
```bash
# Format code
gofmt -w .

# Run tests
go test ./...

# Check for common issues
go vet ./...

# Run linter (if installed)
golangci-lint run
```

---

## Deployment

### Building Release Binaries

```bash
# Build for current platform
make build

# Cross-compile for all platforms
make build-all

# Output:
# bin/cloudunify-darwin-amd64
# bin/cloudunify-darwin-arm64
# bin/cloudunify-linux-amd64
# bin/cloudunify-windows-amd64.exe
```

### Creating Installers

```bash
# macOS .dmg
make package-macos

# Windows installer
make package-windows

# Linux packages
make package-linux
```

### Release Checklist

- [ ] All tests pass
- [ ] Version bumped in `version.go`
- [ ] CHANGELOG.md updated
- [ ] Documentation updated
- [ ] Tested on all platforms
- [ ] Performance benchmarks run
- [ ] Security review completed
- [ ] Binary signatures created

---

## Troubleshooting Guide

### Common Errors

**Error: "FUSE not available"**
- Solution: Install macFUSE/WinFsp/FUSE
- macOS: `brew install macfuse`
- Windows: Download WinFsp installer
- Linux: `sudo apt install fuse`

**Error: "Database locked"**
- Solution: Only one process should access SQLite
- Check for multiple instances running
- Use WAL mode: `PRAGMA journal_mode=WAL`

**Error: "Token expired"**
- Solution: Implement automatic token refresh
- Check refresh token is valid
- Re-authenticate if refresh fails

**Error: "Quota exceeded"**
- Solution: Gracefully handle in provider
- Return clear error to user
- Suggest using different provider

---

## Code Review Checklist

When reviewing code (or having AI review), check:

- [ ] Follows project structure and conventions
- [ ] Error handling is comprehensive
- [ ] Resources are properly closed (defer)
- [ ] Context is passed and respected
- [ ] No sensitive data in logs
- [ ] Tests are included
- [ ] Documentation is updated
- [ ] No SQL injection vulnerabilities
- [ ] Proper concurrency controls (mutexes if needed)
- [ ] Database transactions used where appropriate
- [ ] Efficient algorithms (no O(n²) where O(n) possible)
- [ ] Memory leaks prevented (profile if unsure)

---

## AI Assistant Specific Guidelines

### When User Asks for Implementation

1. **Always check SPEC.md first** - Confirm you understand the architecture
2. **Ask clarifying questions** if requirements are ambiguous
3. **Propose approach** before writing code (let user approve)
4. **Implement incrementally** - Show progress, don't write 1000 lines at once
5. **Write tests** alongside code
6. **Update documentation** if you add/change APIs

### When Debugging

1. **Reproduce the issue** if possible
2. **Check logs** first
3. **Add debug logging** if needed
4. **Use scientific method** - form hypothesis, test, iterate
5. **Explain your reasoning** to the user

### When User is Stuck

1. **Ask about the specific error** message or behavior
2. **Check environment** (OS, Go version, dependencies)
3. **Verify setup** (FUSE installed, providers configured)
4. **Suggest debugging steps** rather than guessing
5. **Point to relevant docs** or examples

### Code Generation Preferences

- ✅ Generate **production-ready** code (with error handling)
- ✅ Include **comments** explaining non-obvious logic
- ✅ Follow **idiomatic Go** patterns
- ✅ Write **testable** code (accept interfaces, not concrete types)
- ❌ Don't use `panic()` except in truly unrecoverable situations
- ❌ Don't use `fmt.Println()` - use proper logging
- ❌ Don't ignore errors (no `err != nil { return nil }`)

---

## Resources

### Documentation
- Go standard library: https://pkg.go.dev/std
- cgofuse: https://github.com/winfsp/cgofuse
- Google Drive API: https://developers.google.com/drive/api/guides/about-sdk
- Microsoft Graph: https://docs.microsoft.com/en-us/graph/api/resources/onedrive

### Tools
- **gofmt** - Code formatting
- **golangci-lint** - Comprehensive linter
- **pprof** - Performance profiling
- **delve** - Debugger

### Learning Resources
- Effective Go: https://go.dev/doc/effective_go
- Go by Example: https://gobyexample.com
- FUSE documentation: https://libfuse.github.io/doxygen/

---

## Version History

- **1.0** (2026-01-08): Initial version

---

## Questions?

If you encounter anything not covered here:
1. Check SPEC.md for technical details
2. Check docs/ folder for additional documentation
3. Ask the user for clarification
4. Update this file with the answer for future reference

---

**Remember:** The goal is to build a reliable, performant, user-friendly cloud storage unification system. When in doubt, prioritize data integrity and user experience over clever code.
