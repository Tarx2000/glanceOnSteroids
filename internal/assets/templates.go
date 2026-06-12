package assets

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

var (
	PageTemplate                  = compileTemplate("page.html", "document.html", "page-style-overrides.gotmpl")
	PageContentTemplate           = compileTemplate("content.html")
	CalendarTemplate              = compileTemplate("calendar.html", "widget-base.html")
	BookmarksTemplate             = compileTemplate("bookmarks.html", "widget-base.html")
	IFrameTemplate                = compileTemplate("iframe.html", "widget-base.html")
	WeatherTemplate               = compileTemplate("weather.html", "widget-base.html")
	ForumPostsTemplate            = compileTemplate("forum-posts.html", "widget-base.html")
	RedditCardsHorizontalTemplate = compileTemplate("reddit-horizontal-cards.html", "widget-base.html")
	RedditCardsVerticalTemplate   = compileTemplate("reddit-vertical-cards.html", "widget-base.html")
	ReleasesTemplate              = compileTemplate("releases.html", "widget-base.html")
	VideosTemplate                = compileTemplate("videos.html", "widget-base.html", "video-card-contents.html")
	VideosGridTemplate            = compileTemplate("videos-grid.html", "widget-base.html", "video-card-contents.html")
	StocksTemplate                = compileTemplate("stocks.html", "widget-base.html")
	RSSListTemplate               = compileTemplate("rss-list.html", "widget-base.html")
	RSSDetailedListTemplate       = compileTemplate("rss-detailed-list.html", "widget-base.html")
	RSSHorizontalCardsTemplate    = compileTemplate("rss-horizontal-cards.html", "widget-base.html")
	RSSHorizontalCards2Template   = compileTemplate("rss-horizontal-cards-2.html", "widget-base.html")
	MonitorTemplate               = compileTemplate("monitor.html", "widget-base.html")
	TwitchGamesListTemplate       = compileTemplate("twitch-games-list.html", "widget-base.html")
	TwitchChannelsTemplate        = compileTemplate("twitch-channels.html", "widget-base.html")
	RepositoryTemplate            = compileTemplate("repository.html", "widget-base.html")
	SpotifyTemplate               = compileTemplate("spotify.html", "widget-base.html")
	CustomAPITemplate             = compileTemplate("custom-api.html", "widget-base.html")
	ClockTemplate                 = compileTemplate("clock.html", "widget-base.html")
	NeuralWattTemplate            = compileTemplate("neuralwatt.html", "widget-base.html")
	ServerStatsTemplate           = compileTemplate("serverstats.html", "widget-base.html")
)

var globalTemplateFunctions = template.FuncMap{
	"relativeTime":      relativeTimeSince,
	"formatViewerCount": formatViewerCount,
	"formatNumber":      intl.Sprint,
	"absInt": func(i int) int {
		return int(math.Abs(float64(i)))
	},
	"formatPrice": func(price float64) string {
		return intl.Sprintf("%.2f", price)
	},
	"formatTime": func(args ...any) string {
		if len(args) == 0 {
			return ""
		}
		var t time.Time
		layout := "2006-01-02 15:04:05"

		if len(args) == 1 {
			if val, ok := args[0].(time.Time); ok {
				t = val
			}
		} else if len(args) >= 2 {
			if lay, ok := args[0].(string); ok {
				layout = lay
			}
			if val, ok := args[1].(time.Time); ok {
				t = val
			}
		}

		if layout == "rfc3339" {
			layout = time.RFC3339
		}
		return t.Format(layout)
	},
	"shouldCollapse": func(i int, collapseAfter int) bool {
		if collapseAfter < -1 {
			return false
		}

		return i >= collapseAfter
	},
	"itemAnimationDelay": func(i int, collapseAfter int) string {
		return fmt.Sprintf("%dms", (i-collapseAfter)*30)
	},
	"dynamicRelativeTimeAttrs": func(t time.Time) template.HTMLAttr {
		return template.HTMLAttr(fmt.Sprintf(`data-dynamic-relative-time="%d"`, t.Unix()))
	},
	"toRelativeTime": func(t time.Time) template.HTMLAttr {
		return template.HTMLAttr(fmt.Sprintf(`data-dynamic-relative-time="%d"`, t.Unix()))
	},
	"now": func() time.Time {
		return time.Now()
	},
	"offsetNow": func(offset string) time.Time {
		dur, err := time.ParseDuration(offset)
		if err != nil {
			return time.Now()
		}
		return time.Now().Add(dur)
	},
	"parseTime": func(layout string, s string) (time.Time, error) {
		if layout == "rfc3339" {
			layout = time.RFC3339
		}
		return time.Parse(layout, s)
	},
	"parseLocalTime": func(layout string, s string) (time.Time, error) {
		if layout == "rfc3339" {
			layout = time.RFC3339
		}
		return time.ParseInLocation(layout, s, time.Local)
	},
	"add": func(a, b any) any {
		return toFloat(a) + toFloat(b)
	},
	"sub": func(a, b any) any {
		return toFloat(a) - toFloat(b)
	},
	"mul": func(a, b any) any {
		return toFloat(a) * toFloat(b)
	},
	"toFloat": func(a any) float64 {
		return toFloat(a)
	},
	"div": func(a, b any) any {
		return toFloat(a) / toFloat(b)
	},
	"newRequest": func(url string) *RequestBuilder {
		return NewRequestBuilder(url)
	},
	"withHeader": func(key, value string, r *RequestBuilder) *RequestBuilder {
		return r.WithHeader(key, value)
	},
	"withStringBody": func(body string, r *RequestBuilder) *RequestBuilder {
		return r.WithStringBody(body)
	},
	"withParameter": func(key, value string, r *RequestBuilder) *RequestBuilder {
		return r.WithParameter(key, value)
	},
	"getResponse": func(r *RequestBuilder) *ResponseWrapper {
		return getResponse(r)
	},
	"safeHTML": func(s string) template.HTML {
		return template.HTML(s)
	},
}

func GlobalTemplateFunctions() template.FuncMap {
	return globalTemplateFunctions
}

func compileTemplate(primary string, dependencies ...string) *template.Template {
	t, err := template.New(primary).
		Funcs(globalTemplateFunctions).
		ParseFS(TemplateFS, append([]string{primary}, dependencies...)...)

	if err != nil {
		panic(err)
	}

	return t
}

var intl = message.NewPrinter(language.English)

func formatViewerCount(count int) string {
	if count < 1_000 {
		return strconv.Itoa(count)
	}

	if count < 10_000 {
		return fmt.Sprintf("%.1fk", float64(count)/1_000)
	}

	if count < 1_000_000 {
		return fmt.Sprintf("%dk", count/1_000)
	}

	return fmt.Sprintf("%.1fm", float64(count)/1_000_000)
}

func relativeTimeSince(t time.Time) string {
	delta := time.Since(t)

	if delta < time.Minute {
		return "1m"
	}
	if delta < time.Hour {
		return fmt.Sprintf("%dm", delta/time.Minute)
	}
	if delta < 24*time.Hour {
		return fmt.Sprintf("%dh", delta/time.Hour)
	}
	if delta < 30*24*time.Hour {
		return fmt.Sprintf("%dd", delta/(24*time.Hour))
	}
	if delta < 12*30*24*time.Hour {
		return fmt.Sprintf("%dmo", delta/(30*24*time.Hour))
	}

	return fmt.Sprintf("%dy", delta/(365*24*time.Hour))
}

// Custom API Request Builder and JSON navigation helpers
var reEnv = regexp.MustCompile(`\${([A-Z_0-9]+)}`)

func expandEnv(s string) string {
	return reEnv.ReplaceAllStringFunc(s, func(m string) string {
		varName := m[2 : len(m)-1]
		return os.Getenv(varName)
	})
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	default:
		return 0
	}
}

type RequestBuilder struct {
	Url     string
	Headers map[string]string
	Params  map[string]string
	Body    string
}

func NewRequestBuilder(url string) *RequestBuilder {
	return &RequestBuilder{
		Url:     url,
		Headers: make(map[string]string),
		Params:  make(map[string]string),
	}
}

func (r *RequestBuilder) WithHeader(key, value string) *RequestBuilder {
	r.Headers[key] = expandEnv(value)
	return r
}

func (r *RequestBuilder) WithStringBody(body string) *RequestBuilder {
	r.Body = expandEnv(body)
	return r
}

func (r *RequestBuilder) WithParameter(key, value string) *RequestBuilder {
	r.Params[key] = expandEnv(value)
	return r
}

type JSONNode struct {
	val any
}

func (n JSONNode) String(path string) string {
	val := queryJSON(n.val, path)
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

func (n JSONNode) Int(path string) int {
	val := queryJSON(n.val, path)
	if val == nil {
		return 0
	}
	switch x := val.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}

func (n JSONNode) Bool(path string) bool {
	val := queryJSON(n.val, path)
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

func (n JSONNode) Array(path string) []JSONNode {
	val := queryJSON(n.val, path)
	if val == nil {
		return nil
	}
	arr, ok := val.([]any)
	if !ok {
		return nil
	}
	nodes := make([]JSONNode, len(arr))
	for i, item := range arr {
		nodes[i] = JSONNode{val: item}
	}
	return nodes
}

func queryJSON(val any, path string) any {
	parts := strings.Split(path, ".")
	curr := val
	for _, part := range parts {
		if curr == nil {
			return nil
		}
		m, ok := curr.(map[string]any)
		if !ok {
			return nil
		}
		curr = m[part]
	}
	return curr
}

type ResponseWrapper struct {
	Response *http.Response
	Body     []byte
	JSON     JSONNode
}

func (r *RequestBuilder) GetResponse() (*ResponseWrapper, error) {
	reqUrl := r.Url
	if len(r.Params) > 0 {
		u, err := url.Parse(r.Url)
		if err == nil {
			q := u.Query()
			for k, v := range r.Params {
				q.Set(k, v)
			}
			u.RawQuery = q.Encode()
			reqUrl = u.String()
		}
	}

	var reqBody io.Reader
	if r.Body != "" {
		reqBody = strings.NewReader(r.Body)
	}

	method := "GET"
	if r.Body != "" {
		method = "POST"
	}

	req, err := http.NewRequest(method, reqUrl, reqBody)
	if err != nil {
		return nil, err
	}

	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonVal any
	_ = json.Unmarshal(respBody, &jsonVal)

	return &ResponseWrapper{
		Response: resp,
		Body:     respBody,
		JSON:     JSONNode{val: jsonVal},
	}, nil
}

func getResponse(r *RequestBuilder) *ResponseWrapper {
	resp, err := r.GetResponse()
	if err != nil {
		return &ResponseWrapper{
			Response: &http.Response{
				StatusCode: 500,
				Status:     err.Error(),
			},
			JSON: JSONNode{val: nil},
		}
	}
	return resp
}
