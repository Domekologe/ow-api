# Building and Deploying on Linux

This guide explains how to build and deploy the Overwatch API on a Linux server.

## Prerequisites

**IMPORTANT:** You must have the **complete source code** on the server to build the binaries. The server needs:
- All `.go` source files
- The `cmd/scraper/` directory with `main.go`
- `go.mod` and `go.sum` files
- All package directories (`ovrstat/`, `cache/`, `config/`, `service/`)

You **cannot** build on the server if you only have the compiled `.exe` files from Windows!

## Building the Binaries

The project consists of **two separate binaries**:
- `api` - The main API server (from `main.go` in root)
- `scraper` - The background scraper service (from `cmd/scraper/main.go`)

### Option 1: Using Make (Recommended)

```bash
# Build both binaries
make build

# Or build individually
make build-api      # Creates 'api' binary
make build-scraper  # Creates 'scraper' binary
```

### Option 2: Manual Build (Without Make)

```bash
# Build API server
go build -o api .

# Build Scraper service
go build -o scraper ./cmd/scraper
```

> **Note:** On Linux, use `api` and `scraper` (no `.exe` extension)

## Uploading Source Code to Server

You have two options to get the source code on the server:

### Option A: Git Clone (Recommended)

```bash
# On the server
cd /opt/api
sudo git clone https://github.com/Domekologe/ow-api.git overwatch
cd overwatch
```

### Option B: Upload via rsync/scp

```bash
# From your local machine (Windows with WSL or Git Bash)
# Replace 'user@server' with your actual server details
rsync -avz --exclude 'cache/' --exclude '*.exe' \
  /w/_Git/ow-api/ user@dks010.domekologe.eu:/opt/api/overwatch/

# Or using scp
scp -r /w/_Git/ow-api user@dks010.domekologe.eu:/opt/api/
```

**Verify the source code is complete:**
```bash
# On the server
ls -la /opt/api/overwatch/cmd/scraper/
# Should show: main.go
```

## Deployment to `/opt/api/overwatch`

### 1. Build the binaries on the server

```bash
cd /path/to/ow-api
make build
```

### 2. Copy files to deployment directory

```bash
# Create deployment directory
sudo mkdir -p /opt/api/overwatch

# Copy binaries
sudo cp api scraper /opt/api/overwatch/

# Copy configuration files
sudo cp .env config.yaml /opt/api/overwatch/

# Create cache directory
sudo mkdir -p /opt/api/overwatch/cache

# Set ownership (adjust user:group as needed)
sudo chown -R voice:voice /opt/api/overwatch

# Set execute permissions
sudo chmod +x /opt/api/overwatch/api
sudo chmod +x /opt/api/overwatch/scraper
```

### 3. Install systemd service files

```bash
# Copy service files
sudo cp .example-api-service /etc/systemd/system/owapi.service
sudo cp .example-scraper-service /etc/systemd/system/owapi-scraper.service

# Reload systemd
sudo systemctl daemon-reload

# Enable services to start on boot
sudo systemctl enable owapi.service
sudo systemctl enable owapi-scraper.service

# Start services
sudo systemctl start owapi.service
sudo systemctl start owapi-scraper.service
```

### 4. Verify services are running

```bash
# Check API service status
sudo systemctl status owapi.service

# Check Scraper service status
sudo systemctl status owapi-scraper.service

# View logs
sudo journalctl -u owapi.service -f
sudo journalctl -u owapi-scraper.service -f
```

## Troubleshooting

### Exit Code 203/EXEC

This error means systemd cannot execute the binary. Common causes:

1. **Binary doesn't exist**: Verify both `api` and `scraper` exist in `/opt/api/overwatch/`
   ```bash
   ls -la /opt/api/overwatch/
   ```

2. **Missing execute permissions**:
   ```bash
   sudo chmod +x /opt/api/overwatch/api
   sudo chmod +x /opt/api/overwatch/scraper
   ```

3. **Wrong user/group**: Ensure the `voice` user exists and has access
   ```bash
   id voice
   sudo chown -R voice:voice /opt/api/overwatch
   ```

4. **Missing dependencies**: Ensure all required libraries are installed
   ```bash
   ldd /opt/api/overwatch/api
   ldd /opt/api/overwatch/scraper
   ```

### Service Won't Start

Check the systemd journal for detailed error messages:
```bash
sudo journalctl -u owapi-scraper.service -n 50 --no-pager
```

### Redis Connection Issues

Ensure Redis is running and accessible:
```bash
sudo systemctl status redis.service
redis-cli ping
```

## Quick Deployment Script

```bash
#!/bin/bash
# deploy.sh - Quick deployment script

set -e

echo "Building binaries..."
make build

echo "Stopping services..."
sudo systemctl stop owapi.service owapi-scraper.service || true

echo "Copying binaries..."
sudo cp api scraper /opt/api/overwatch/

echo "Setting permissions..."
sudo chown voice:voice /opt/api/overwatch/api /opt/api/overwatch/scraper
sudo chmod +x /opt/api/overwatch/api /opt/api/overwatch/scraper

echo "Starting services..."
sudo systemctl start owapi.service owapi-scraper.service

echo "Checking status..."
sudo systemctl status owapi.service --no-pager
sudo systemctl status owapi-scraper.service --no-pager

echo "✓ Deployment complete!"
```

Make it executable:
```bash
chmod +x deploy.sh
```
