# CloudUnify

A unified cloud storage system that mounts multiple cloud providers (Google Drive, OneDrive, iCloud) as a single virtual filesystem.

## Prerequisites

- **Go 1.21+** - `brew install go`
- **Node.js 18+** - `brew install node`
- **macFUSE** (for mounting virtual filesystem) - `brew install --cask macfuse`
  - After installation, approve the kernel extension in System Settings > Privacy & Security
  - Restart your Mac

## Quick Start

### 1. Build the Backend

```bash
# From the project root
make build
```

Or manually:
```bash
go build -o bin/cloudunify ./cmd/cloudunify
```

### 2. Run the Backend

```bash
./bin/cloudunify
```

The API server will start on `http://localhost:8080`

### 3. Run the Frontend (Development)

```bash
cd web
npm install
npm run dev
```

The web UI will be available at `http://localhost:5173`

## Available Commands

### Makefile Targets

```bash
make build       # Build the backend binary
make build-dev   # Build with debug symbols
make run         # Build and run the backend
make clean       # Remove build artifacts
make test        # Run tests
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check |
| `/api/providers` | GET | List connected providers |
| `/api/providers` | POST | Add a new provider |
| `/api/providers/:id` | DELETE | Remove a provider |
| `/api/storage` | GET | Get storage statistics |
| `/api/files` | GET | List files in directory |
| `/api/sync/queue` | GET | Get sync queue status |
| `/ws` | WebSocket | Real-time updates |

## Project Structure

```
cloud-storage-sync/
├── cmd/cloudunify/       # Application entry point
├── internal/
│   ├── api/              # HTTP API server & WebSocket
│   ├── config/           # Configuration management
│   ├── database/         # SQLite database layer
│   ├── fuse/             # Virtual filesystem (FUSE)
│   ├── providers/        # Cloud provider implementations
│   ├── storage/          # Storage allocation strategy
│   └── sync/             # Sync engine & queue
├── web/                  # React frontend (Vite)
├── bin/                  # Compiled binaries
└── Makefile
```

## Configuration

Configuration is stored in:
- **macOS**: `~/Library/Application Support/CloudUnify/config.json`
- **Linux**: `~/.config/cloudunify/config.json`
- **Windows**: `%APPDATA%\CloudUnify\config.json`

### Default Settings

```json
{
  "mount_path": "~/CloudUnify",
  "cache": {
    "enabled": true,
    "max_size_gb": 10
  },
  "sync": {
    "upload_workers": 3,
    "download_workers": 5,
    "auto_sync": true
  },
  "api": {
    "host": "localhost",
    "port": 8080
  }
}
```

## Development

### Backend Development

```bash
# Run with hot reload (using air or similar)
make run

# Run tests
go test ./...
```

### Frontend Development

```bash
cd web
npm run dev      # Start dev server with HMR
npm run build    # Build for production
npm run preview  # Preview production build
```

## Notes

- **macFUSE** requires a system restart after installation
- OAuth credentials for cloud providers need to be configured for production use
- The first pass implements a working skeleton - provider authentication uses mock tokens

## License

MIT
