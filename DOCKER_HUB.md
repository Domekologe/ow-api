# 📦 Repository Overview — owapi

***

**owapi** is a lightweight and efficient API server written in Go for **Overwatch** stats, built with the Echo web framework. It provides a clean, ultra-fast interface to retrieve player statistics. This project now includes a dedicated **Background Scraper** service to keep data fresh and reduce load times via Redis caching.

***

## 🚀 Features

- **Fast and minimal:** Written in Go for performance and simplicity.
- **Dual Services:** Includes both an **API** service and a **Background Scraper**.
- **Caching:** Built-in Redis support for caching responses and reducing external requests.
- **Echo framework:** Structured routing & middleware support.
- **Container-ready:** Prebuilt Docker images for both API and Scraper services.
- **Portable:** Works across Linux, macOS and Windows environments.
- **Production-ready defaults:** Alpine-based final images for small footprint.

***

## 📁 Repository

**Source:** [https://github.com/Domekologe/ow-api](https://github.com/Domekologe/ow-api)

The source contains:
- 🚧 Full Go source code for API and Scraper
- 📄 Go module and dependency definitions
- 📦 Dockerfile with multi-stage build (creating both `api` and `scraper` images)
- 📌 API logic and server entrypoint

***

## 🐳 Docker Images

This repository provides two main images:

### 1. API Server
The main API application that serves player stats.

**Image name:**
```
domekologe/owapi:latest
```

**Pull with:**
```bash
docker pull domekologe/owapi:latest
```

**Run (Standalone):**
```bash
docker run -p 8080:8080 domekologe/owapi:latest
```

### 2. Scraper Service
A background service that automatically updates cached player data.

**Image name:**
```
domekologe/owapi:scraper-latest
```

**Pull with:**
```bash
docker pull domekologe/owapi:scraper-latest
```

***

## 🧠 Technical Details

- **Language:** Go (Module aware)
- **Framework:** Echo v4
- **Build:** Static binary via Go build (Multi-stage Dockerfile)
- **Final image:** Alpine Linux w/ ca-certificates & tzdata
- **Security:** Runs as non-privileged user (nobody)
- **No CGO:** Built with `CGO_ENABLED=0` for portability

***

## ⚙️ Typical Usage

Use these images to:
- Deploy a self-hosted Overwatch API microservice.
- Serve HTTP APIs with high availability (using Redis).
- Keep data fresh automatically with the background scraper.

***

## 📌 Example: Docker Compose

Ideally, you should run the **API**, **Scraper**, and **Redis** together. Here is a complete `docker-compose.yml` example:

```yaml
version: '3.8'

services:
  # Redis Database (Required for Caching & Scraper)
  redis:
    image: redis:7-alpine
    container_name: ow-api-redis
    restart: unless-stopped
    volumes:
      - redis-data:/data

  # API Service
  api:
    image: domekologe/owapi:latest
    container_name: ow-api
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - REDIS_ENABLED=true
      - REDIS_HOST=redis
      - CACHE_TTL=24h
      - API_TIMEOUT=5s
    depends_on:
      - redis

  # Scraper Service (Background updates)
  scraper:
    image: domekologe/owapi:scraper-latest
    container_name: ow-api-scraper
    restart: unless-stopped
    environment:
      - REDIS_ENABLED=true
      - REDIS_HOST=redis
      - SCRAPER_ENABLED=true
      - SCRAPER_INTERVAL=60m
    depends_on:
      - redis

# Persistent volume for Redis data
volumes:
  redis-data:
    driver: local
```

***

## ❓ Notes

- **Environment Variables:** Ensure variables like `REDIS_HOST` match your deployment setup.
- **Scraper:** The scraper requires Redis to be enabled (`REDIS_ENABLED=true`) to function.
- **Tags:** Images are tagged as `:latest` (API) and `:scraper-latest` (Scraper). Version tags are also available (e.g., `:v1.0.0` and `:scraper-v1.0.0`).

***

## ⚖️ Disclaimer

This project is not affiliated with, endorsed by, or associated with **Blizzard Entertainment, Inc.**

“Overwatch” is a registered trademark of Blizzard Entertainment, Inc.

This project is an independent, community-driven effort.
