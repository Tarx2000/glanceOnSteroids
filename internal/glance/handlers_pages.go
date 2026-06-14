package glance

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/glanceapp/glance/internal/assets"
)

// HandlePageRequest handles loading the dashboard HTML shell.
func (a *Application) HandlePageRequest(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	page, exists := a.slugToPage[r.PathValue("page")]
	a.configMu.RUnlock()

	if !exists {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Trigger asynchronous background pre-fetch
	go page.UpdateOutdatedWidgets(a.Hub)

	pageData := templateData{
		Page: page,
		App:  a,
	}

	var responseBytes bytes.Buffer
	err := assets.PageTemplate.Execute(&responseBytes, pageData)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Write(responseBytes.Bytes())
}

// HandlePageContentRequest compiles the HTML layout columns and widgets of a page.
func (a *Application) HandlePageContentRequest(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	page, exists := a.slugToPage[r.PathValue("page")]
	a.configMu.RUnlock()

	if !exists {
		a.HandleNotFound(w, r)
		return
	}

	// Trigger asynchronous background pre-fetch
	go page.UpdateOutdatedWidgets(a.Hub)

	pageData := templateData{
		Page: page,
	}

	var responseBytes bytes.Buffer
	err := assets.PageContentTemplate.Execute(&responseBytes, pageData)

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Write(responseBytes.Bytes())
}

// HandleNotFound renders a basic not found response.
func (a *Application) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte("Page not found"))
}

// HandlePageAdd appends a new page to glance.yml.
func (a *Application) HandlePageAdd(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload.Name = strings.TrimSpace(payload.Name)
	if payload.Name == "" {
		http.Error(w, "page name cannot be empty", http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.AddPage(payload.Name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandlePageDelete deletes a page from glance.yml.
func (a *Application) HandlePageDelete(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024*1024)

	var payload struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload.Slug = strings.TrimSpace(payload.Slug)
	if payload.Slug == "" {
		http.Error(w, "page slug cannot be empty", http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.DeletePage(payload.Slug); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// HandleConfigImport overwrites the entire config file.
func (a *Application) HandleConfigImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)

	err := r.ParseMultipartForm(2 * 1024 * 1024)
	if err != nil {
		http.Error(w, "failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing or invalid file upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".yml") &&
		!strings.HasSuffix(strings.ToLower(header.Filename), ".yaml") {
		http.Error(w, "only .yml or .yaml files are accepted", http.StatusBadRequest)
		return
	}

	contentBytes, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read file: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := a.ConfigManager.ImportConfig(contentBytes); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}



	slog.Info("Config imported successfully", "filename", header.Filename)
	w.WriteHeader(http.StatusOK)
}
