# AGENTS.md

## Project Context

This is a personal enhanced fork of [Glance](https://github.com/glanceapp/glance), a self-hosted dashboard application written in Go.
It features a layout editor with drag-and-drop, real-time Spotify playback via WebSockets, and dynamic widget management.

## Development Conventions

- **Language:** Go 1.25+ backend, vanilla JS/CSS frontend.
- **No external JS frameworks:** Keep frontend minimal and dependency-free.
- **Screen reader / Accessibility improvements:** Intentionally excluded. This is a personal project and accessibility enhancements (e.g., ARIA labels, skip-links, keyboard-only DnD) are not a priority.
- **Database:** SQLite via `modernc.org/sqlite`.
- **Config:** `glance.yml` is the single source of truth; all runtime mutations write back to it while preserving structure/comments via YAML AST.
- **Widget Modal Completeness & UI/UX:** When creating or modifying widgets, it is critical to ensure that all configuration properties supported by the widget are fully exposed in both the "Add Widget" and "Edit Widget" modals. Keep the inputs clean, modern, and intuitive, prioritizing simple and highly responsive UI/UX.
- **Widget Template Rendering Safety:** In shared templates (specifically `widget-base.html`), never evaluate fields that are specific to certain widget types without checking for their availability (or defining them in the base `widgetBase` struct in `internal/widget/widget.go`). Go's `html/template` package fails hard when evaluating missing fields on structs, causing the entire widget to return empty HTML and disappear from the page.

## Known Architectural Notes

- **Widget concurrency:** Page updates spawn goroutines per widget. A semaphore (`maxConcurrentUpdates = 5`) is used to bound concurrency and prevent resource exhaustion.
- **YAML AST mutations:** All config edits manipulate `yaml.Node` directly to preserve comments and formatting. After editing, `saveNodeToDisk` serializes with 2-space indentation.
- **WebSocket state:** Spotify polling is paused when `ActiveConnections() == 0` to save battery on low-spec clients.
- **Hot reload:** `reloadConfig()` attempts to match old widget cached states to new widget configs by serializing both to YAML and comparing strings.

## Build Version & Cache Busting

- **Build Number:** Stored in `BUILD_NUMBER` file at project root. Increment this number **every time** you make changes to the codebase (frontend or backend). This number is injected via `-ldflags` at build time and displayed in the footer as `v{version}-b{buildNumber}`.
- **Cache busting:** Static asset URLs (`main.css`, `main.js`) include `?v=b{buildNumber}-{timestamp}` to bust browser caches on every rebuild.
- **Important:** Because frontend assets are embedded via `go:embed`, you **must rebuild the binary and restart** for any JS/CSS/HTML changes to take effect. Simply restarting the old binary will serve stale assets.
- **When making changes:** Bump `BUILD_NUMBER` by 1, then rebuild and restart.
