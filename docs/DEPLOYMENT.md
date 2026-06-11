# VPS Deployment Guide

Dieses Dokument beschreibt, wie du GlanceOnSteroids auf einem VPS mit Docker deployst und über Tailscale erreichbar machst.

## Voraussetzungen

- VPS mit Docker & Docker Compose
- Tailscale installiert auf dem VPS
- Git

## Schnellstart

```bash
# 1. Repo klonen
git clone https://github.com/Tarx2000/glanceOnSteroids.git
cd glanceOnSteroids

# 2. Config aus Template erstellen
cp config/glance.yml.template config/glance.yml
# → config/glance.yml nach Bedarf anpassen

# 3. Docker Image bauen (Multi-Stage)
docker build -t glance-on-steroids:latest -f Dockerfile.fullbuild .

# 4. Container starten
docker compose up -d
```

Das Dashboard ist dann unter `http://VPS_IP:8081` erreichbar.

## Multi-Stage Build

Da auf dem VPS kein Go installiert ist, baut `Dockerfile.fullbuild` das Binary direkt im Container:

| Stage | Basis | Zweck |
|-------|-------|-------|
| Builder | `golang:1.24-alpine` | Go-Modul-Download & Binary-Kompilierung |
| Runtime | `alpine:3.19` | Finales, minimales Image (~20 MB) |

```bash
# Build mit expliziter Build-Nummer
docker build \
  --build-arg BUILD_VERSION=v1.0 \
  --build-arg BUILD_NUMBER=50 \
  -t glance-on-steroids:latest \
  -f Dockerfile.fullbuild .
```

## Docker Compose

Die `docker-compose.yml` ist für VPS-Deployment vorkonfiguriert:

```yaml
services:
  glance-dashboard:
    build:
      context: .
      dockerfile: Dockerfile.single-platform
    container_name: glance-dashboard
    network_mode: bridge
    volumes:
      - ./config:/data          # Config & SQLite DB
      - /var/run/docker.sock:/var/run/docker.sock:ro  # Docker-Status
      - /etc/timezone:/etc/timezone:ro
      - /etc/localtime:/etc/localtime:ro
    ports:
      - "8081:8080"             # Host-Port 8081 → Container-Port 8080
    restart: unless-stopped
```

**Wichtige Details:**
- **Port 8081** statt 8080 (falls 8080 bereits belegt ist)
- **Docker-Socket** wird gemountet für das Server-Stats Widget (zeigt Container-Status)
- **Config** liegt unter `./config/glance.yml` und wird nach `/data/glance.yml` gemountet
- **SQLite-DB** (`glance.db`) wird im gleichen Verzeichnis angelegt

## Tailscale Exposition

### Option A: Direkter Zugriff über Tailscale IP

Wenn dein VPS im Tailnet ist, reicht der direkte Zugriff:

```
http://100.x.x.x:8081
```

### Option B: Tailscale Domain + HTTPS (empfohlen)

Damit das Dashboard über `https://vmdxxxxx.taildxxxxx.ts.net/` erreichbar ist:

```bash
# Tailscale Serve aktivieren (Port 443 → localhost:8081)
tailscale serve --https=443 http://localhost:8081
```

**Alternative mit Dokploy/Traefik:** Falls auf dem VPS bereits Dokploy mit Traefik läuft:

1. Tailscale-Machine-Zertifikate nutzen:
   ```bash
   # Zertifikate finden
   ls /var/lib/tailscale/certs/*.ts.net*
   ```

2. Traefik Dynamic-Config erstellen (`glance.yml`):
   ```yaml
   http:
     routers:
       glance-tailscale:
         rule: "Host(`vmdxxxxx.taildxxxxx.ts.net`)"
         service: glance-service
         tls:
           options: default
     services:
       glance-service:
         loadBalancer:
           servers:
             - url: "http://localhost:8081"
   ```

3. In Traefik-Container deployen:
   ```bash
   docker cp glance.yml dokploy-traefik:/etc/dokploy/traefik/dynamic/
   docker restart dokploy-traefik
   ```

## Konfiguration

### Config-Datei

Die `glance.yml` wird beim ersten Start automatisch erstellt, falls sie nicht existiert. Für das VPS-Setup ist diese Config empfohlen:

```yaml
server:
  host: ""              # WICHTIG: Leerer String = alle Interfaces (nicht nur localhost!)
  port: 8080
  timezone: Europe/Berlin

pages:
  - name: Home
    columns:
      - size: small
        widgets:
          - type: server-stats
            hide-title: true
```

**Achtung:** `host: ""` ist essentiell! Bei `host: 127.0.0.1` ist das Dashboard nur innerhalb des Containers erreichbar und die Seite bleibt leer.

### Widgets

| Widget | Beschreibung | Config |
|--------|-------------|--------|
| `server-stats` | CPU, RAM, Disk, Docker, Uptime | Keine Config nötig |
| `spotify` | Aktuelle Wiedergabe | `client-id`, `client-secret`, `redirect-url` |
| `neuralwatt` | LLM-Token-Usage | `api-key` (OpenRouter) |
| `weather` | Wetter | `location`, `units` |

## Update-Prozess

```bash
# 1. Neue Version pullen
git pull origin main

# 2. Image neu bauen
docker build -t glance-on-steroids:latest -f Dockerfile.fullbuild .

# 3. Container neu starten (Config & DB bleiben erhalten)
docker compose down
docker compose up -d

# 4. Prüfen
curl -s http://localhost:8081 | head -5
```

**Tipp:** Da Config und DB per Volume außerhalb des Containers liegen (`./config:/data`), gehen beim Rebuild keine Daten verloren.

## Troubleshooting

### Seite bleibt leer / Connection refused

```bash
# Prüfen ob Container läuft
docker ps | grep glance

# Logs prüfen
docker logs glance-dashboard

# Config prüfen: host MUSS "" sein
grep "host:" config/glance.yml
```

### Docker-Status zeigt "N/A"

Der Docker-Socket muss gemountet sein:

```yaml
volumes:
  - /var/run/docker.sock:/var/run/docker.sock:ro
```

Außerdem muss `docker-cli` im Image installiert sein (bereits in `Dockerfile.single-platform` enthalten).

### Port 8081 ist belegt

Alternative Ports in `docker-compose.yml`:

```yaml
ports:
  - "8082:8080"
```

## Sicherheitshinweise

- Die `config/glance.yml` enthält ggf. API-Keys (Spotify, OpenRouter). Sie ist in `.gitignore` eingetragen und wird **nicht** committed.
- Für git-basierte Backups: `config/glance.yml.template` nutzen und Secrets separat verwalten.
- Tailscale-Domains (`*.ts.net`) sind nur innerhalb deines Tailnet erreichbar — kein öffentlicher Zugriff.
