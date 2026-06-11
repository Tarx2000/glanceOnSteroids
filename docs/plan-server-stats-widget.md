# Server-Stats Widget — Implementation Plan

> **For:** Taris GlanceOnSteroids Fork
> **Goal:** Ein Widget, das Live-Server-Stats (CPU, RAM, Disk, Docker, Uptime) direkt im Dashboard anzeigt
> **Architecture:** Selbst-contained Widget im bestehenden `widget`-Package; liest lokale System-Dateien (`/proc`, `df`) und Docker-Status; rendert über Go HTML-Template mit CSS-Bars
> **Tech Stack:** Go 1.25+, vanilla CSS, Docker CLI (exec), keine externen Abhängigkeiten

---

## Overview

Dieses Widget zeigt in Echtzeit (alle 30 Sekunden aktualisiert):

- **CPU-Auslastung** in Prozent (gestapelte Bar: user/system/idle)
- **RAM-Auslastung** in Prozent (used/total in GB)
- **Disk-Auslastung** in Prozent (used/total in GB)
- **Docker-Container** Status (running / stopped / total)
- **System-Uptime** (Tage, Stunden, Minuten)

Die Daten werden lokal aus dem Container gelesen — kein externer API-Call nötig.

---

## Files to Touch (Summary)

| # | File | Action | Purpose |
|---|------|--------|---------|
| 1 | `internal/widget/widget.go:22-64` | Modify | Widget-Typ "server-stats" in Factory registrieren |
| 2 | `internal/widget/serverstats.go` | **Create** | Widget-Logik: CPU, RAM, Disk, Docker, Uptime parsen |
| 3 | `internal/assets/templates/serverstats.html` | **Create** | HTML-Template mit progress bars |
| 4 | `internal/assets/templates.go:45-46` | Modify | Template-Variable `ServerStatsTemplate` registrieren |
| 5 | `internal/assets/static/main.css` | Modify | CSS-Klassen für Server-Stats Bars und Grid |
| 6 | `internal/assets/static/main.js` | Modify | Formular-Definition für Layout-Editor |
| 7 | `internal/assets/templates/page.html:103-110` | Modify | Dropdown-Option im Widget-Picker |

---

## Task 1: Widget-Factory erweitern

**Objective:** "server-stats" als neuen Widget-Typ in `widget.New()` registrieren.

**File:** `internal/widget/widget.go:22-64`

**Step 1: Neuen Case hinzufügen**

Füge in der `switch`-Anweisung in `widget.New()` einen neuen Case ein (z.B. nach `case "neuralwatt":`):

```go
case "server-stats":
    return &ServerStats{}, nil
```

**Verification:**
```bash
grep -n 'server-stats' internal/widget/widget.go
# Expected output:
# 59:	case "server-stats":
# 60:		return &ServerStats{}, nil
```

**Commit:**
```bash
git add internal/widget/widget.go
git commit -m "feat: register server-stats widget type in factory"
```

---

## Task 2: Widget-Struktur und Logik erstellen

**Objective:** `ServerStats`-Struct mit `Initialize`, `Update`, `Render` implementieren.

**File:** `internal/widget/serverstats.go` (neu erstellen)

**Step 1: Datei erstellen**

```go
package widget

import (
	"bufio"
	"context"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// ServerStats zeigt CPU, RAM, Disk, Docker und Uptime des Hosts
type ServerStats struct {
	widgetBase `yaml:",inline"`

	// Runtime-Daten (werden in Update() gesetzt)
	CPUUser   int     `yaml:"-"`
	CPUSystem int     `yaml:"-"`
	CPUIdle   int     `yaml:"-"`
	RAMUsedGB float64 `yaml:"-"`
	RAMTotalGB float64 `yaml:"-"`
	RAMPercent int    `yaml:"-"`
	DiskUsedGB float64 `yaml:"-"`
	DiskTotalGB float64 `yaml:"-"`
	DiskPercent int    `yaml:"-"`
	DockerRunning int  `yaml:"-"`
	DockerStopped int  `yaml:"-"`
	DockerTotal int    `yaml:"-"`
	UptimeHours int    `yaml:"-"`
	UptimeMinutes int  `yaml:"-"`
	UptimeDays int     `yaml:"-"`
}

func (widget *ServerStats) Initialize() error {
	widget.withTitle("Server Stats")
	widget.withCacheDuration(30 * time.Second)
	return nil
}

func (widget *ServerStats) Update(ctx context.Context) {
	widget.readCPU()
	widget.readRAM()
	widget.readDisk()
	widget.readDocker()
	widget.readUptime()
	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *ServerStats) Render() template.HTML {
	return widget.render(widget, assets.ServerStatsTemplate)
}

// ─── CPU aus /proc/stat ────────────────────────────────────────────────

func (widget *ServerStats) readCPU() {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)[1:]
			if len(fields) < 4 {
				continue
			}
			user, _ := strconv.Atoi(fields[0])
			nice, _ := strconv.Atoi(fields[1])
			system, _ := strconv.Atoi(fields[2])
			idle, _ := strconv.Atoi(fields[3])
			iowait, _ := strconv.Atoi(fields[4])

			total := user + nice + system + idle + iowait
			if total > 0 {
				widget.CPUUser = ((user + nice) * 100) / total
				widget.CPUSystem = (system * 100) / total
				widget.CPUIdle = (idle * 100) / total
			}
			break
		}
	}
}

// ─── RAM aus /proc/meminfo ─────────────────────────────────────────────

func (widget *ServerStats) readRAM() {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var memTotal, memAvailable int
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fmt.Sscanf(line, "MemTotal: %d kB", &memTotal)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			fmt.Sscanf(line, "MemAvailable: %d kB", &memAvailable)
		}
	}
	if memTotal > 0 {
		widget.RAMTotalGB = float64(memTotal) / 1024 / 1024
		used := memTotal - memAvailable
		widget.RAMUsedGB = float64(used) / 1024 / 1024
		widget.RAMPercent = (used * 100) / memTotal
	}
}

// ─── Disk aus df ───────────────────────────────────────────────────────

func (widget *ServerStats) readDisk() {
	cmd := exec.Command("df", "-B1", "/")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 6 {
		return
	}
	total, _ := strconv.ParseInt(fields[1], 10, 64)
	used, _ := strconv.ParseInt(fields[2], 10, 64)
	widget.DiskTotalGB = float64(total) / 1024 / 1024 / 1024
	widget.DiskUsedGB = float64(used) / 1024 / 1024 / 1024
	if total > 0 {
		widget.DiskPercent = int((used * 100) / total)
	}
}

// ─── Docker Container Status ───────────────────────────────────────────

func (widget *ServerStats) readDocker() {
	// Running containers
	cmd := exec.Command("docker", "ps", "-q")
	out, err := cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			widget.DockerRunning = 0
		} else {
			widget.DockerRunning = len(lines)
		}
	}

	// All containers
	cmd = exec.Command("docker", "ps", "-aq")
	out, err = cmd.Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 1 && lines[0] == "" {
			widget.DockerTotal = 0
		} else {
			widget.DockerTotal = len(lines)
		}
	}
	widget.DockerStopped = widget.DockerTotal - widget.DockerRunning
}

// ─── Uptime aus /proc/uptime ───────────────────────────────────────────

func (widget *ServerStats) readUptime() {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return
	}
	seconds, _ := strconv.ParseFloat(fields[0], 64)
	totalMinutes := int(seconds) / 60
	widget.UptimeDays = totalMinutes / 60 / 24
	widget.UptimeHours = (totalMinutes / 60) % 24
	widget.UptimeMinutes = totalMinutes % 60
}
```

**Verification:**
```bash
# Kompiliert es?
go build ./internal/widget/
# Expected: no errors
```

**Commit:**
```bash
git add internal/widget/serverstats.go
git commit -m "feat: add ServerStats widget logic (cpu, ram, disk, docker, uptime)"
```

---

## Task 3: HTML-Template erstellen

**Objective:** Visuelles Template mit Progress-Bars für jede Metrik.

**File:** `internal/assets/templates/serverstats.html` (neu erstellen)

```html
{{ template "widget-base.html" . }}

{{ define "widget-content" }}
<div class="server-stats-widget">

  <!-- CPU -->
  <div class="ss-row">
    <div class="ss-label">
      <span>CPU</span>
      <span class="ss-value">{{ .CPUUser }}% user / {{ .CPUSystem }}% sys</span>
    </div>
    <div class="ss-bar-bg">
      <div class="ss-bar-fill ss-bar-cpu-user" style="width: {{ .CPUUser }}%"></div>
      <div class="ss-bar-fill ss-bar-cpu-system" style="width: {{ .CPUSystem }}%; left: {{ .CPUUser }}%"></div>
    </div>
  </div>

  <!-- RAM -->
  <div class="ss-row">
    <div class="ss-label">
      <span>RAM</span>
      <span class="ss-value">{{ printf "%.1f" .RAMUsedGB }} / {{ printf "%.1f" .RAMTotalGB }} GB</span>
    </div>
    <div class="ss-bar-bg">
      <div class="ss-bar-fill ss-bar-ram" style="width: {{ .RAMPercent }}%"></div>
    </div>
  </div>

  <!-- Disk -->
  <div class="ss-row">
    <div class="ss-label">
      <span>Disk</span>
      <span class="ss-value">{{ printf "%.1f" .DiskUsedGB }} / {{ printf "%.1f" .DiskTotalGB }} GB</span>
    </div>
    <div class="ss-bar-bg">
      <div class="ss-bar-fill ss-bar-disk" style="width: {{ .DiskPercent }}%"></div>
    </div>
  </div>

  <!-- Docker + Uptime in einer Zeile -->
  <div class="ss-grid-2">
    <div class="ss-card">
      <div class="ss-card-icon">🐳</div>
      <div class="ss-card-value">{{ .DockerRunning }} / {{ .DockerTotal }}</div>
      <div class="ss-card-label">Containers (running)</div>
      {{ if gt .DockerStopped 0 }}
      <div class="ss-card-sub">{{ .DockerStopped }} stopped</div>
      {{ end }}
    </div>
    <div class="ss-card">
      <div class="ss-card-icon">⏱</div>
      <div class="ss-card-value">{{ .UptimeDays }}d {{ .UptimeHours }}h</div>
      <div class="ss-card-label">Uptime</div>
    </div>
  </div>

</div>
{{ end }}
```

**Verification:**
```bash
ls -la internal/assets/templates/serverstats.html
# Expected: file exists with 89 lines
```

**Commit:**
```bash
git add internal/assets/templates/serverstats.html
git commit -m "feat: add serverstats html template with progress bars"
```

---

## Task 4: Template in Go registrieren

**Objective:** `ServerStatsTemplate`-Variable in `templates.go` anlegen.

**File:** `internal/assets/templates.go:21-46`

**Step 1: Neue Variable hinzufügen**

Füge unter `NeuralWattTemplate` (Zeile 45) eine neue Zeile ein:

```go
	NeuralWattTemplate            = compileTemplate("neuralwatt.html", "widget-base.html")
	ServerStatsTemplate           = compileTemplate("serverstats.html", "widget-base.html")
```

**Verification:**
```bash
grep -n 'ServerStatsTemplate' internal/assets/templates.go
# Expected output:
# 46:	ServerStatsTemplate           = compileTemplate("serverstats.html", "widget-base.html")
```

**Commit:**
```bash
git add internal/assets/templates.go
git commit -m "feat: register ServerStatsTemplate in templates.go"
```

---

## Task 5: CSS-Styling hinzufügen

**Objective:** Progress-Bar-Styles und Grid-Layout für Server-Stats.

**File:** `internal/assets/static/main.css`

**Step 1: Am Ende der Datei anhängen**

```css
/* ─── Server-Stats Widget ─────────────────────────────────────────── */

.server-stats-widget {
  padding: 0;
}

.ss-row {
  margin-bottom: 12px;
}

.ss-row:last-of-type {
  margin-bottom: 0;
}

.ss-label {
  display: flex;
  justify-content: space-between;
  font-size: 0.85rem;
  margin-bottom: 4px;
  opacity: 0.9;
}

.ss-value {
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.ss-bar-bg {
  position: relative;
  height: 6px;
  background: var(--color-widget-content-border);
  border-radius: 3px;
  overflow: hidden;
}

.ss-bar-fill {
  position: absolute;
  height: 100%;
  border-radius: 3px;
  transition: width 0.3s ease;
}

.ss-bar-cpu-user {
  background: linear-gradient(90deg, #4ade80, #22c55e);
  z-index: 2;
}

.ss-bar-cpu-system {
  background: linear-gradient(90deg, #fbbf24, #f59e0b);
  z-index: 1;
}

.ss-bar-ram {
  background: linear-gradient(90deg, #60a5fa, #3b82f6);
}

.ss-bar-disk {
  background: linear-gradient(90deg, #a78bfa, #8b5cf6);
}

.ss-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
  margin-top: 14px;
}

.ss-card {
  background: var(--color-background);
  border: 1px solid var(--color-widget-content-border);
  border-radius: 8px;
  padding: 10px;
  text-align: center;
}

.ss-card-icon {
  font-size: 1.2rem;
  margin-bottom: 4px;
}

.ss-card-value {
  font-size: 1.1rem;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}

.ss-card-label {
  font-size: 0.75rem;
  opacity: 0.7;
  margin-top: 2px;
}

.ss-card-sub {
  font-size: 0.7rem;
  color: #ef4444;
  margin-top: 2px;
}
```

**Verification:**
```bash
grep -n 'server-stats-widget' internal/assets/static/main.css
# Expected output:
# 1:.server-stats-widget {
```

**Commit:**
```bash
git add internal/assets/static/main.css
git commit -m "feat: add server-stats widget css (progress bars, grid cards)"
```

---

## Task 6: Layout-Editor Formular-Definition

**Objective:** Im JS-Layout-Editor kann man "Server Stats" als Widget-Typ auswählen.

**File:** `internal/assets/static/main.js`

**Step 1: Im `widgetForms`-Objekt neue Definition hinzufügen**

Suche im Code nach dem `neuralwatt`-Entry (ca. Zeile 1090). Füge **danach** einen neuen Eintrag hinzu:

```javascript
    serverstats: `
        <p style="font-size: 0.85em; opacity: 0.7; margin-bottom: 10px;">Zeigt CPU, RAM, Disk, Docker und Uptime des Servers an.</p>
    `,
```

**Step 2: Im Widget-Type Picker registrieren**

Suche nach der Stelle, wo `neuralwatt` als Option im Dropdown angeboten wird (in der JS-Datei, nicht HTML). Füge `serverstats` hinzu.

**Alternative (falls dynamisch aus HTML generiert):** Fahre mit Task 7 fort.

**Verification:**
```bash
grep -n 'serverstats' internal/assets/static/main.js
# Expected output:
# 1:    serverstats: `
```

**Commit:**
```bash
git add internal/assets/static/main.js
git commit -m "feat: add serverstats widget form definition in layout editor"
```

---

## Task 7: Dropdown-Option im Page-Template

**Objective:** "Server Stats" erscheint im "Add Widget"-Dropdown der Seite.

**File:** `internal/assets/templates/page.html:103-110`

**Step 1: Neue `<option>` einfügen**

Füge nach `<option value="neuralwatt">NeuralWatt Usage</option>` eine neue Zeile ein:

```html
<option value="server-stats">Server Stats</option>
```

**Verification:**
```bash
grep -n 'server-stats' internal/assets/templates/page.html
# Expected output:
# 109:	<option value="server-stats">Server Stats</option>
```

**Commit:**
```bash
git add internal/assets/templates/page.html
git commit -m "feat: add server-stats to widget type dropdown"
```

---

## Task 8: Integration Test (lokal bauen)

**Objective:** Gesamtes Feature kompiliert und zeigt Daten an.

**Step 1: Vollständigen Build testen**

```bash
# Im Projekt-Root
go build -o /tmp/glance-test ./
# Expected: no errors
```

**Step 2: Config-Beispiel**

```yaml
pages:
  - name: Home
    columns:
      - size: small
        widgets:
          - type: server-stats
```

**Step 3: Dashboard starten und prüfen**

```bash
/tmp/glance-test --config ./config/glance.yml
# Im Browser öffnen: http://localhost:8080
```

**Ergebnis prüfen:**
- Widget-Titel "Server Stats" sichtbar
- CPU-Bar bewegt sich bei Last
- RAM zeigt verwendeten/totalen Speicher
- Disk zeigt verwendeten/totalen Platz
- Docker zeigt Anzahl laufender Container
- Uptime zeigt Tage und Stunden

**Commit:**
```bash
git commit -m "feat: server-stats widget integration complete"
```

---

## Task 9: BUILD_NUMBER bumpen (wichtig!)

**Objective:** Cache-Busting für Frontend-Assets.

**File:** `BUILD_NUMBER`

**Step 1: Zahl um 1 erhöhen**

```bash
echo "35" > BUILD_NUMBER
```

**Verification:**
```bash
cat BUILD_NUMBER
# Expected: 35
```

**Commit:**
```bash
git add BUILD_NUMBER
git commit -m "chore: bump BUILD_NUMBER for cache busting"
```

---

## Optional: Erweiterungen (nicht im MVP)

Diese Features sind **nicht** im initialen Plan, können aber später einfach ergänzt werden:

- **Temperature**: `/sys/class/thermal/thermal_zone*/temp` auslesen
- **Network I/O**: `/proc/net/dev` parsen
- **Top-Prozesse**: `ps aux --sort=-%cpu | head -6`
- **Multi-Disk**: Mehrere Mountpoints (`df / /home /var`)
- **History-Sparkline**: Letzte 10 CPU-Werte als Mini-Chart

---

## Verification Checklist (final)

- [ ] `go build ./...` kompiliert ohne Fehler
- [ ] `widget.New("server-stats")` gibt einen validen Widget-Pointer zurück
- [ ] Page-Template enthält `<option value="server-stats">Server Stats</option>`
- [ ] `ServerStatsTemplate` ist in `templates.go` registriert
- [ ] `main.js` enthält `serverstats:` Formular-Definition
- [ ] `main.css` enthält `.server-stats-widget` Styles
- [ ] `BUILD_NUMBER` wurde erhöht
- [ ] Dashboard rendert das Widget ohne Template-Fehler
- [ ] Alle 5 Metriken (CPU, RAM, Disk, Docker, Uptime) zeigen plausible Werte

---

## Architecture Notes

### Warum `os/exec` statt Docker SDK?

- Keine neue Dependency (Docker SDK wäre ein neues Go-Modul)
- `docker ps` funktioniert out-of-the-box wenn Container mit `-v /var/run/docker.sock:/var/run/docker.sock` läuft
- Falls der Host keinen Docker-Socket hat, zeigt das Widget einfach 0 Container an (graceful degradation)

### Thread-Safety

- `widgetBase` hat bereits ein `sync.Mutex`
- `Update()` läuft seriell pro Widget (durch `glance.go` Threadpool garantiert)
- Alle `read*()`-Methoden schreiben auf `widget.*` Fields — kein externer Shared State

### Performance

- Cache-Dauer: 30 Sekunden (genug für Live-Feeling, nicht zu viel System-Load)
- `/proc/*` Lesen ist sub-millisecond Operation
- `df` und `docker ps` sind ebenfalls schnell (< 50ms)
- Keine Netzwerk-Calls, keine externen APIs

---

## Deliverables

Am Ende dieses Plans existieren:

1. **9 Commits** (einer pro Task)
2. **2 neue Dateien:** `serverstats.go`, `serverstats.html`
3. **5 modifizierte Dateien:** `widget.go`, `templates.go`, `main.css`, `main.js`, `page.html`
4. **1 ge-bump-te Datei:** `BUILD_NUMBER`
5. **Funktionierendes Widget** mit Live-Server-Daten
