# Gorgon

**Gorgon** is a self-hosted media automation tool for tracking and managing TV shows.

Built in **Go** with a **HTML + HTMX** frontend, Gorgon allows users to automatically search, monitor, and organize episodes through a clean web interface. It integrates with torrent clients and indexers to automate the download and organization of content. Similar to projects like Sonarr or Pymedusa.

---

## 🧠 Key Features

- 📺 Track TV shows with metadata from TVMaze (automatic background sync every 6h with smart cleanup)
- 🌸 **Anime support & auto-detection**: Automatic anime classification from genres, dedicated anime search patterns, absolute episode numbering (`- 12`, `Ep 12`, `[12]`), and CRC32 hash stripping
- 🔍 Search for episodes via Prowlarr indexers with standard and season-less release matching
- 💾 Automate downloads with qBittorrent integration, including live speed monitoring and direct pause/resume torrent controls
- ✈️ **Telegram notifications & bot**: Instant alerts for snatched and downloaded episodes, scheduled daily summaries, and interactive on-demand commands (`/summary`, `/ping`, `/start`)
- 🧹 Organize downloads into structured folders with symlinks
- 🎛️ Keyword-based **filter engine**: profiles with `required` / `rejected` / `preferred` (scored) patterns, per-show search patterns and global defaults
- 🏷️ Custom aliases per show, searched alongside the canonical name
- 🕒 **Timezone awareness**: Configurable application timezone with searchable selector; automatically localizes calendar, air times, and daily summaries
- ⚡ Live UI updates via WebSocket and Idiomorph DOM morphing (episode tracking buttons update in real time with zero flicker)
- 📅 Calendar page with upcoming episode releases
- 📥 Downloads page with active progress, live transfer speeds, category filtering, and torrent state controls
- 📝 File-based logging with a dedicated Logs page
- 🧠 Background scheduler with graceful shutdown, periodic sync, cleanup, and update routines
- 💻 Modern responsive Web UI built with HTML + HTMX (mobile navigation drawer, refined theme, glassmorphic toast notifications)
- 📖 Open API docs served at `/api/v1/docs` (Swagger)

---

## 📸 Screenshots

![Shows Page](https://i.imgur.com/fFum9P2.png)

---

## 📦 Stack

- **Backend**: Go 1.26+ (single binary, no runtime dependencies)
- **Frontend**: HTML + HTMX (server-rendered with progressive enhancement)
- **Database**: SQLite (lightweight and embedded)
- **Torrent Client**: [qBittorrent](https://www.qbittorrent.org/)
- **Indexer Integration**: [Prowlarr](https://github.com/Prowlarr/Prowlarr)
- **Scheduler/Jobs**: Custom cron-based workers
- **Notifications**: Telegram Bot Api

> ❗ Note: Gorgon does not provide or host any content. You must configure your own torrent client and indexer.

---

## ⚙️ Requirements

- Go 1.26+ (to build from source)
- [qBittorrent](https://www.qbittorrent.org/) (with Web UI enabled)
- [Prowlarr](https://github.com/Prowlarr/Prowlarr) (torrent/usanet indexer aggregator)
- ~50MB disk space (SQLite database + logs)

---

## 🐳 Docker & Docker Compose

You can run Gorgon and its dependencies (Prowlarr, qBittorrent) using Docker Compose for a quick and easy setup.

1.  **Create a `docker-compose.yml` file:**

    ```yaml
    services:
      gorgon:
        container_name: gorgon
        image: ghcr.io/jusoaresg/gorgon:latest
        ports:
          - "8080:8080"
        volumes:
          - path/to/gorgon/configs:/configs
          - path/to/your/downloads:/downloads
          - path/to/your/shows:/shows
        networks:
          - gorgon-network

      prowlarr:
          container_name: prowlarr
          image: ghcr.io/hotio/prowlarr
          ports:
            - "9696:9696"
          environment:
            - PUID=1000
            - PGID=1000
            - UMASK=002
            - TZ=America/Sao_Paulo
          volumes:
            - path/to/your/prowlarr/config:/config
          restart: unless-stopped
          networks:
              - gorgon-network

      qbittorrent:
        container_name: qbittorrent
        image: lscr.io/linuxserver/qbittorrent:latest
        environment:
          - PUID=1000
          - PGID=1000
          - TZ=Etc/UTC
          - WEBUI_PORT=9191
          - TORRENTING_PORT=6881
        volumes:
          - path/to/gorgon/downloads:/downloads
          - path/to/your/torrent/config:/config
        ports:
          - 9191:9191
          - 6881:6881
          - 6881:6881/udp
        restart: unless-stopped
        networks:
            - gorgon-network

    networks:
      gorgon-network:
        driver: bridge
    ```

2.  **Run it:**

    ```bash
    docker-compose up -d
    ```

This will build the Gorgon image, pull the required dependencies, and start all services.

- **Gorgon** will be available at `http://localhost:8080`
- **Prowlarr** will be available at `http://localhost:9696`
- **qBittorrent** will be available at `http://localhost:9191`

### 🐳 Images

Pre-built images are published automatically to the [GitHub Container Registry](https://github.com/Jusoaresg/gorgon/pkgs/container/gorgon) when pushing to `main` or creating a release:

- `ghcr.io/jusoaresg/gorgon:latest` — built from `main`
- `ghcr.io/jusoaresg/gorgon:vX.Y.Z` (and `vX.Y`) — built from version tags / releases

Multi-architecture images are provided for `linux/amd64` and `linux/arm64`. The container listens on port `8080` by default (override with the `GORGON_PORT` env var). Other env vars: `GORGON_BASE_DIR` (base path for the `configs`, `downloads` and `shows` folders) and `IN_DOCKER=true` (set automatically in the official image).

### ⚙️ Initial Setup

-   **Prowlarr**:
    1.  Open Prowlarr at `http://localhost:9696`.
    2.  Go to **Settings > General** and copy your **API Key**.
    3.  In Gorgon's UI, go to the configurations page and paste the API key to connect to Prowlarr.

-   **qBittorrent**:
    1.  On the first run, qBittorrent will generate a random password. Check the logs for the `qbittorrent` container to find it: `docker-compose logs qbittorrent`.
    2.  The default username is `admin`.
    3.  Log in to the qBittorrent Web UI at `http://localhost:9191`.
    4.  Go to **Tools > Options > Web UI** and change the username and password.
    5.  In Gorgon's UI, go to the configurations page and enter your new credentials.

-   **General & Timezone**:
    1.  Go to **Settings > General**.
    2.  Select your local timezone using the searchable combobox so calendar airings and Telegram daily summaries align with your local time.

---

## 🎛️ Filtering

Gorgon filters search results and release candidates with **profiles**, **per-show search patterns**, and **intelligent episode matching**.

### Filter Profiles

Profiles are reusable collections of **gates** and a search pattern, configured in **Settings → Filters** and shared across shows:

- `search` — the query template(s) used on Prowlarr
- `required` — words the release filename must contain
- `rejected` — words that disqualify a release
- `preferred` — words that add a configurable **score** weighing how strongly a release is wanted

A default profile can be set globally and applied to every show. Each profile can define its own search patterns, and the `preferred` patterns carry a score field.

### Defaults

Under **Settings → Filters**, global search behavior can be configured for every show:

- **Default filter profile** — applied to shows without their own profile (saves automatically when changed)
- **Use aliases when searching** — search releases using the show's aliases too
- **Ignore non-latin aliases** — skip aliases containing non-latin characters

### Per-show search patterns

Each show (via the **Edit Series** modal) can list its own **search patterns**, for when a specific show needs a particular query style. These are **combined** (and deduplicated) with the selected profile's search patterns — each one becomes a Prowlarr query. The default `{alias} S{season:00}E{episode:00}` is **always searched first** (when not already listed) so the most precise episode query runs before the extra patterns.

### Placeholders

Patterns may use placeholders that are replaced with the show's data:

| Placeholder | Meaning |
| --- | --- |
| `{alias}` | Show name or alias used for the search |
| `{show}` | Canonical show name |
| `{season}` | Season number |
| `{episode}` | Episode number |
| `{season:00}` | Season number zero-padded |
| `{episode:00}` | Episode number zero-padded |

### 🌸 Anime & Season-less Releases

Gorgon automatically detects anime series when added (via TVMaze genre/type tags) and can be toggled manually per show (`standard` vs `anime`):

- **Auto-Prepended Patterns**: For anime shows, Gorgon automatically prepends `{alias} - {episode:00}` and `{alias} S{season:00} - {episode:00}` before general queries.
- **Flexible Token Matching**:
  - Standard notations: `S01E02`, `1x02`, `S01.E02`, `S01E02-E04`, standalone `E02`, `Episode 02`, `Ep. 02`, `Part 02`.
  - Anime notations: `Show - 12`, `Show - 05v2`, `Show Ep 12`, `Show [12]`.
- **CRC32 Checksum Stripping**: Automatically removes bracketed 8-character release hashes (e.g. `[26EF07AE]`) so checksum digits do not trigger false episode matches.
- **Smart Season-less Fallback**: Releases lacking an explicit season tag default strictly to **Season 1**. When searching for Season 2+, season-less releases are safely rejected to prevent cross-season false positives.

---

## 🤖 Telegram Bot & Notifications

Gorgon includes native Telegram bot integration for real-time notifications, automated daily schedule overviews, and interactive bot commands (`/summary`, `/ping`, `/start`) without requiring open inbound ports or webhooks.

### 🔔 Event Notifications

- **Episode Snatched**: Alerts when an episode release is grabbed and queued for download in qBittorrent, including indexer info and an inline **🔗 View Release** button.
- **Episode Downloaded**: Alerts when the download completes and the episode is moved and organized.
- **Lock-Screen Optimized**: The notification title is placed first (`✅ Downloaded: Show Name` / `📥 Snatched: Show Name`) so mobile and desktop push previews display immediate context.

### 📅 Scheduled Daily Summary

- **Automated Morning Overview**: Sends an automated daily summary of episodes scheduled to air each day at your configured local time (e.g., `08:00`).
- **Clean Tree Layout**: Displays show titles, episode codes, air times, and download statuses (`✅ Downloaded`, `📥 Snatched`, `⏳ Wanted`, `🔍 Missing`) in a structured tree format.

---

## 📡 API

Gorgon exposes a REST API under `/api/v1`. Live Swagger documentation is available at:

- `/api/v1/docs` — interactive docs UI
- `/swagger.json` — OpenAPI 2.0 spec

The web UI consumes this same API, so every interaction in the interface maps to a documented endpoint.

---

## 🚧 Status

**v0.3** — under active development, but usable for personal setups.

Current highlights: the filter engine (profiles, per-show patterns, scoring), anime auto-detection & absolute numbering, custom aliases, live UI updates via WebSocket, full search → download → organize automation, and native Telegram bot integration.

---

## 🗺️ Roadmap

Here are the next steps planned for Gorgon, focusing on expanding features, improving usability, and ensuring robustness.

### 🎯 Core Functionality
- **Search & Filtering:**
  - [X] Enable automatic and manual search triggers for individual episodes.
  - [X] Implement "Search All Missing" functionality on the show page.
  - [X] Implement the keyword-based scoring system for search results.
  - [X] Add filter profiles and per-show search patterns (combined and deduplicated).
  - [X] Support anime auto-detection, absolute numbering, and CRC checksum stripping.
  - [X] Support standalone episode matching with safe Season 1 fallback.
- **Organization & Tracking:**
  - [X] Add a "Downloads" page with live speed metrics, category filters, and pause/resume torrent controls.
  - [X] Add per-show custom aliases.
  - [ ] Add a "Bulk Edit" feature for managing multiple shows at once.
- **User Interface:**
  - [X] Create a "Calendar" page to display upcoming episode releases for tracked shows.
  - [X] Persist the user's choice of Grid or List view on the shows page.
  - [X] Update episode buttons in real time via WebSocket.
  - [X] Configurable application timezone with localized calendar and release dates.
  - [X] Responsive mobile UI with drawer navigation and glassmorphic toast notifications.
- **System & Management:**
  - [X] Implement file-based logging with a dedicated page in the UI for viewing logs.
  - [X] Periodic TVMaze sync every 6 hours with cleanup of unsnatched removed episodes.
  - [X] Centralized scheduler manager with graceful shutdown and context cancellation.

### 🔌 Integrations
- **Notifications & Bots:**
  - [X] Telegram bot integration (snatch & download push alerts, scheduled daily summary, and interactive bot listener).
- **Torrent Clients:**
  - [ ] Add support for µTorrent.
  - [ ] Add support for Transmission.
  - [ ] Add support for Deluge.
- **Indexers:**
  - [ ] Add support for Jackett as an alternative to Prowlarr.
- **Automation:**
  - [ ] Make the RSS feed worker honor filter profiles and per-show search patterns.

### 🧪 Development & DevOps
- **Testing:**
  - [X] Comprehensive unit test coverage across the backend (services, repositories, filter engine, and race-tested scheduler).
- **Deployment:**
  - [X] Create a `Dockerfile` for the Gorgon application.
  - [x] Set up a `docker-compose.yml` file for a complete, one-command deployment with Prowlarr and a torrent client.
  - [X] Implement a CI pipeline for automated testing and builds.
  - [X] Automatically publish Docker images to GHCR on `main` and releases (multi-arch).
  - [X] Automate GitHub Releases with binaries via GoReleaser.
  - [X] Dependabot for Go and GitHub Actions dependency updates.
  - [X] Security scanning with CodeQL and `govulncheck`.

## 📜 Disclaimer

> Gorgon is a personal open-source project for media organization and automation.  
> It **does not host, provide, or encourage access to copyrighted content**.  
> The user is solely responsible for how they configure and use the software.

---

## 🧑‍💻 Author

**Juliano Soares San Gregorio**  
[GitHub](https://github.com/jusoaresg) · [LinkedIn](https://linkedin.com/in/juliano-gregorio)
