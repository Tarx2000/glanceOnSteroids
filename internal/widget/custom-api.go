package widget

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"time"

	"github.com/glanceapp/glance/internal/assets"
)

// CustomAPI fetches data from an external API and renders it using a user-supplied Go template.
type CustomAPI struct {
	widgetBase         `yaml:",inline"`
	URL                string            `yaml:"url"`
	Method             string            `yaml:"method"`
	Headers            map[string]string `yaml:"headers"`
	AllowInsecure      BoolField         `yaml:"allow-insecure"`
	SkipJSONValidation BoolField         `yaml:"skip-json-validation"`
	Template           string            `yaml:"template"`
	ParsedData         interface{}       `yaml:"-"`
	CachedHTML         string            `yaml:"-"`
	parsedTempl        *template.Template `yaml:"-"`
	httpClient         *http.Client      `yaml:"-"`
}

func init() {
	Register("custom-api", func() Widget { return &CustomAPI{} })
}

func (widget *CustomAPI) Initialize() error {
	widget.withTitle("Custom API").withCacheDuration(time.Minute)

	if widget.Method == "" {
		widget.Method = "GET"
	}

	tmpl, err := template.New("custom-api-user").
		Funcs(assets.GlobalTemplateFunctions()).
		Parse(widget.Template)
	if err != nil {
		return fmt.Errorf("failed compiling custom-api template: %w", err)
	}
	widget.parsedTempl = tmpl

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if widget.AllowInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	widget.httpClient = &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	return nil
}

func (widget *CustomAPI) Update(ctx context.Context, services ExternalServiceProvider) {
	if widget.parsedTempl == nil {
		widget.withError(fmt.Errorf("template not compiled"))
		return
	}

	data := map[string]interface{}{
		"Data":    widget.ParsedData,
		"URL":     widget.URL,
		"Method":  widget.Method,
		"Headers": widget.Headers,
	}

	if widget.URL != "" {
		req, err := http.NewRequestWithContext(ctx, widget.Method, widget.URL, nil)
		if err != nil {
			widget.canContinueUpdateAfterHandlingErr(err)
			return
		}

		for k, v := range widget.Headers {
			req.Header.Set(k, v)
		}
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Glance-CustomAPI/1.0")
		}

		resp, err := widget.httpClient.Do(req)
		if err != nil {
			widget.canContinueUpdateAfterHandlingErr(err)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			widget.canContinueUpdateAfterHandlingErr(err)
			return
		}

		data["StatusCode"] = resp.StatusCode
		data["Body"] = string(body)

		if widget.SkipJSONValidation {
			widget.ParsedData = string(body)
			data["Data"] = widget.ParsedData
		} else {
			var parsed interface{}
			if err := json.Unmarshal(body, &parsed); err != nil {
				widget.canContinueUpdateAfterHandlingErr(fmt.Errorf("invalid JSON response: %w", err))
				return
			}
			widget.ParsedData = parsed
			data["Data"] = parsed
		}
	}

	var buf bytes.Buffer
	if err := widget.parsedTempl.Execute(&buf, data); err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	widget.Lock()
	widget.CachedHTML = buf.String()
	widget.Unlock()

	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *CustomAPI) RenderHTML() template.HTML {
	widget.Lock()
	defer widget.Unlock()
	return template.HTML(widget.CachedHTML)
}

func (widget *CustomAPI) Render() template.HTML {
	return widget.render(widget, assets.CustomAPITemplate)
}
