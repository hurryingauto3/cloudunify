# CloudUnify

A unified cloud storage system that mounts multiple cloud providers (Google Drive, OneDrive, iCloud) as a single virtual filesystem at `~/CloudUnify`. Simply drag-and-drop files into the folder and they automatically sync to the cloud.

## Features

- 🗂️ **Unified Mount Point** - All cloud storage appears as a single `~/CloudUnify` folder
- 📤 **Drag-and-Drop Upload** - Copy files via Finder or terminal, they upload automatically
- ☁️ **Google Drive Integration** - OAuth 2.0 with resumable uploads for large files
- 🔄 **Background Sync** - Multi-worker queue with retry logic
- 📊 **Web Dashboard** - Real-time progress, storage visualization
- 🖥️ **Native Performance** - FUSE-based with local staging for fast writes

## Prerequisites

- **Go 1.21+** - `brew install go`
- **Node.js 18+** - `brew install node`
- **macFUSE** - `brew install --cask macfuse`
  - After installation, approve the kernel extension in System Settings > Privacy & Security
  - **Restart your Mac**

## Quick Start

### 1. Configure Google Drive OAuth

Create `~/.cloudunify.env` or set environment variables:
```bash
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
```

### 2. Build & Run

```bash
# Build the backend
make build

# Run CloudUnify
./bin/cloudunify
```

### 3. Authenticate

1. Open `http://localhost:8080` in your browser
2. Click "Add Provider" → "Google Drive"
3. Complete OAuth authentication
4. Start copying files to `~/CloudUnify`!

### 4. Optional: Web Dashboard

```bash
cd web
npm install
npm run dev
```

Access the dashboard at `http://localhost:5173`

## Usage

### Copy Files via Terminal
```bash
cp ~/Downloads/video.mp4 ~/CloudUnify/
ls -la ~/CloudUnify/
```

### Copy Files via Finder
Simply drag and drop files into the `~/CloudUnify` folder in Finder.

### Check Upload Status
```bash
# View sync queue
curl http://localhost:8080/api/sync/queue | jq

# View all files
curl http://localhost:8080/api/files | jq
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check |
| `/api/providers` | GET | List connected providers |
| `/api/providers` | POST | Add a new provider |
| `/api/providers/:id` | DELETE | Remove a provider |
| `/api/files` | GET | List all synced files |
| `/api/files/upload` | POST | Direct file upload |
| `/api/sync/queue` | GET | Get sync queue status |
| `/api/oauth/google/start` | GET | Start Google OAuth flow |
| `/ws` | WebSocket | Real-time sync updates |

## Project Structure

```
cloud-storage-sync/
├── cmd/cloudunify/       # Application entry point
├── internal/
│   ├── api/              # HTTP API server & WebSocket
│   ├── config/           # Configuration management
│   ├── database/         # SQLite database layer
│   ├── fuse/             # FUSE virtual filesystem
│   ├── providers/        # Cloud provider implementations
│   ├── storage/          # Storage allocation strategy
│   └── sync/             # Background sync engine
├── web/                  # React frontend (Vite)
├── bin/                  # Compiled binaries
└── Makefile
```

## Configuration

Configuration stored at `~/Library/Application Support/CloudUnify/`:
- `config.json` - Settings and OAuth tokens
- `cloudunify.db` - SQLite metadata database

Cache/staging at `~/Library/Caches/CloudUnify/staging/`

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
    "download_workers": 5
  },
  "api": {
    "port": 8080
  }
}
```

## Development

### Build Commands

```bash
make build       # Build the backend binary
make build-dev   # Build with debug symbols
make run         # Build and run the backend
make clean       # Remove build artifacts
make test        # Run tests
```

### Frontend Development

```bash
cd web
npm run dev      # Start dev server with HMR
npm run build    # Build for production
```

## Troubleshooting

### "Mount point busy" error
```bash
umount ~/CloudUnify  # or: diskutil unmount force ~/CloudUnify
```

### Files not uploading
1. Check provider is registered: `curl http://localhost:8080/api/providers`
2. Check sync queue: `curl http://localhost:8080/api/sync/queue`
3. Check logs: `cat /tmp/cloudunify.log`

### OAuth errors
Ensure `GOOGLE_CLIENT_ID` and `GOOGLE_CLIENT_SECRET` are set before starting.

## Current Status

✅ **Working:**
- FUSE filesystem mount at ~/CloudUnify
- Drag-and-drop file upload via Finder
- Terminal file copy (`cp`)
- Google Drive OAuth & upload
- Background sync with progress
- Web dashboard

🚧 **Planned:**
- OneDrive integration
- iCloud integration
- Smart file distribution
- Download on read

## License

MIT
