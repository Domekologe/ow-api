<p align="center">
<img src="service/static/assets/logo.png" width="310" height="71" border="0" alt="ovrstat">
<br>
<a href="https://goreportcard.com/badge/github.com/Domekologe/ow-api"><img src="https://goreportcard.com/badge/github.com/Domekologe/ow-api" alt="Go Report Card"></a>
<a href="https://pkg.go.dev/badge/github.com/Domekologe/ow-api"><img src="https://pkg.go.dev/badge/github.com/Domekologe/ow-api" alt="GoDoc"></a>
</p>

## Latest Changes
ATTENTION!
With the latest update `namecardID` and `namecardTitle` are removed! (Thanks to Blizzard for again making changes)

## General
After ovrstat is obsolete/archived and OW-API didn't get specific values I made an functional version here for my own.

`ovrstat` is a simple web scraper for the Overwatch stats site that parses and serves the data retrieved as JSON. Included is the go package used to scrape the info for usage in any go binary. This is a single endpoint web-scraping API that takes the full payload of information that we retrieve from Blizzard and passes it through to you in a single response. Things like caching and splitting data across multiple responses could likely improve performance, but in pursuit of keeping things simple, ovrstat does not implement them.

## Configuration
The application can be configured via environment variables or a `config.yaml` file:

| Variable | Description | Default |
| :--- | :--- | :--- |
| `PORT` | The HTTP port the server listens on | `8080` |
| `REDIS_ENABLED` | Enable Redis caching | `false` |
| `REDIS_HOST` | Redis server hostname | `localhost` |
| `REDIS_PORT` | Redis server port | `6379` |
| `REDIS_PASSWORD` | Redis password (if required) | `` |
| `REDIS_DB` | Redis database number | `0` |
| `CACHE_TTL` | How long to cache player data | `24h` |
| `API_TIMEOUT` | Timeout before using cache fallback | `5s` |
| `SCRAPER_ENABLED` | Enable background scraper | `false` |
| `SCRAPER_INTERVAL` | How often to update cached data | `60m` |
| `ADMIN_PASSWORD` | Password for admin endpoints | `` (disabled) |
| `DEBUG` | Enable verbose debug logging | `false` |

### Configuration Files: `config.yaml` vs `.env`

**Important:** There are two ways to configure the application:

#### 1. `config.yaml` (For Local Development)
- Automatically loaded on startup (if present)
- Ideal for local development
- **Not** committed to Git (see `.gitignore`)
- Example configuration file included in repository

```yaml
server:
  port: "8080"
redis:
  enabled: true
  host: "localhost"
  port: 6379
admin:
  password: "my-secure-password"
```

#### 2. Environment Variables (For Docker/Production)
- Override `config.yaml` values
- Ideal for Docker and production deployments
- `.env` file is just an **example** (see `.env.example`)
- In Docker: Set variables in `docker-compose.yml` or as system environment variables

**Priority:** Environment Variables > config.yaml > Defaults

### Admin Endpoints

When `ADMIN_PASSWORD` is set, the following protected endpoints are available:

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/admin/cache/flush` | POST | Clears the entire Redis cache |
| `/admin/scraper/trigger` | POST | Shows scraper info (Note: Scraper runs separately) |
| `/admin/cache/stats` | GET | Shows cache statistics |

**Authentication:**
```bash
# Flush cache
curl -X POST http://localhost:8080/admin/cache/flush \
  -H "Authorization: Bearer your-admin-password"

# Get cache statistics
curl http://localhost:8080/admin/cache/stats \
  -H "Authorization: Bearer your-admin-password"
```

## Installation & Usage

### 1. Build from Source
Ensure you have [Go](https://go.dev/dl/) installed (minimum 1.24).

```bash
# Clone the repository
git clone https://github.com/Domekologe/ow-api.git
cd ow-api

# Build both binaries using Makefile
make build

# Or build individually:
make build-api      # Creates api.exe (API Server)
make build-scraper  # Creates scraper.exe (Background Scraper)

# Or manually with go build (Windows without Make):
go build -o api.exe .                    # API Server
go build -o scraper.exe ./cmd/scraper    # Background Scraper
```

> **Note:** On Windows, `make` is not available by default. Use the `go build` commands directly.

**Which binary for what?**
- **`api.exe`** - Main server (HTTP API on port 8080)
- **`scraper.exe`** - Background service for automatic cache updates (runs separately)

### 2. Run the Application

**API Server (Without Redis):**
```bash
./api.exe
# or
make run
```

**With Redis (Local):**
```bash
# Redis must be running (e.g. via Docker: docker run -d -p 6379:6379 redis:7-alpine)
REDIS_ENABLED=true REDIS_HOST=localhost ./api.exe
```

**Scraper Service (Separate):**
```bash
# Requires Redis
REDIS_ENABLED=true REDIS_HOST=localhost SCRAPER_ENABLED=true ./scraper.exe
# or
make run-scraper
```

The server will start on port 8080 (default).

### 3. Docker Compose (Recommended)
The easiest way to run the complete stack with Redis caching and background scraper:

```bash
# Set admin password (optional but recommended)
export ADMIN_PASSWORD="your-secure-password"

# Start all services (Redis, API, Scraper)
docker-compose up -d

# View logs
docker-compose logs -f

# Stop all services
docker-compose down

# Stop and remove volumes (clears Redis data)
docker-compose down -v
```

The API will be available at `http://localhost:8080` with Redis caching enabled.

**Using Admin Endpoints:**
```bash
curl -X POST http://localhost:8080/admin/cache/flush \
  -H "Authorization: Bearer your-secure-password"
```

### 4. Docker (Single Container)
You can run the official image directly:

```bash
docker run -p 8080:8080 domekologe/owapi:latest
```

Or build it locally:
```bash
docker build -t owapi --target api .
docker run -p 8080:8080 owapi
```

**Note:** Running a single container without Redis will work but won't have caching enabled.

## How It Works

### Caching Strategy
1. **First Request**: API scrapes Blizzard's site (slow, ~2-5 seconds)
2. **Cached Requests**: Instant response from Redis
3. **Timeout Fallback**: If Blizzard is slow (>5s), returns cached data
4. **Background Updates**: Scraper refreshes all cached players every hour

### Architecture
- **API Service**: Handles HTTP requests with Redis caching
- **Scraper Service**: Periodically updates cached player data
- **Redis**: Stores player stats with 24h TTL

## API Usage

Below is an example of using the REST endpoint (note: CASE matters for the username/tag):
```
http://localhost:8080/stats/pc/Viz-1213
http://localhost:8080/stats/console/Viz-1213
```

### Using Go to retrieve Stats

```go
package main

import (
	"log"

	"github.com/Domekologe/ow-api/ovrstat"
)

func main() {
	log.Println(ovrstat.Stats(ovrstat.PlatformPC, "Viz-1213"))
    log.Println(ovrstat.Stats(ovrstat.PlatformConsole, "Viz-1213"))
}
```

## Disclaimer
ovrstat isn’t endorsed by Blizzard and doesn’t reflect the views or opinions of Blizzard or anyone officially involved in producing or managing Overwatch. Overwatch and Blizzard are trademarks or registered trademarks of Blizzard Entertainment, Inc. Overwatch © Blizzard Entertainment, Inc.

The BSD 3-clause License
========================

Copyright (c) 2023, s32x, Domekologe, ToasterUwU. All rights reserved.

Redistribution and use in source and binary forms, with or without modification,
are permitted provided that the following conditions are met:

 - Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.

 - Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

 - Neither the name of ovrstat nor the names of its contributors may
   be used to endorse or promote products derived from this software without
   specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR
ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON
ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
