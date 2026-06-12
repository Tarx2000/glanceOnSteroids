package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"gopkg.in/yaml.v3"
)

// RSSFeedItem represents a single parsed entry from an RSS feed.
type RSSFeedItem struct {
	ChannelName     string    `json:"channel-name"`
	ChannelURL      string    `json:"channel-url"`
	Title           string    `json:"title"`
	Link            string    `json:"link"`
	ImageURL        string    `json:"image-url"`
	PublishedAt     time.Time `json:"published-at"`
	Description     string    `json:"description,omitempty"`
	Categories      []string  `json:"categories,omitempty"`
	HideCategories  bool      `json:"hide-categories,omitempty"`
	HideDescription bool      `json:"hide-description,omitempty"`
}

// RSSFeedRequest defines the configuration for an individual RSS feed subscription.
type RSSFeedRequest struct {
	Url             string            `yaml:"url"`
	Title           string            `yaml:"title"`
	HideCategories  bool              `yaml:"hide-categories"`
	HideDescription bool              `yaml:"hide-description"`
	Limit           *int              `yaml:"limit,omitempty"`
	ItemLinkPrefix  string            `yaml:"item-link-prefix"`
	Headers         map[string]string `yaml:"headers,omitempty"`
}

// UnmarshalYAML allows decoding an RSS feed request either from a plain URL string or a structured map.
func (r *RSSFeedRequest) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		r.Url = node.Value
		return nil
	}
	type plain RSSFeedRequest
	return node.Decode((*plain)(r))
}

type RSSFeedItems []RSSFeedItem

// SortByNewest sorts RSS items in descending chronological order (newest first).
func (f RSSFeedItems) SortByNewest() RSSFeedItems {
	sort.Slice(f, func(i, j int) bool {
		return f[i].PublishedAt.After(f[j].PublishedAt)
	})

	return f
}

var feedParser = gofeed.NewParser()
var htmlTagRegex = regexp.MustCompile("<[^>]*>")

// stripHTML removes HTML tags and decodes basic entities to yield clean plain text.
func stripHTML(src string) string {
	str := htmlTagRegex.ReplaceAllString(src, "")
	str = strings.ReplaceAll(str, "&amp;", "&")
	str = strings.ReplaceAll(str, "&lt;", "<")
	str = strings.ReplaceAll(str, "&gt;", ">")
	str = strings.ReplaceAll(str, "&quot;", "\"")
	str = strings.ReplaceAll(str, "&#39;", "'")
	str = strings.ReplaceAll(str, "&nbsp;", " ")
	return strings.TrimSpace(str)
}

// getItemsFromRSSFeedTask fetches and parses a single RSS feed.
func getItemsFromRSSFeedTask(request RSSFeedRequest) ([]RSSFeedItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	var f *gofeed.Feed
	var err error

	// If custom headers are provided, execute the request manually using http.Client
	if len(request.Headers) > 0 {
		client := &http.Client{Timeout: 5 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", request.Url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range request.Headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		f, err = feedParser.Parse(resp.Body)
	} else {
		f, err = feedParser.ParseURLWithContext(request.Url, ctx)
	}

	if err != nil {
		return nil, err
	}

	items := make(RSSFeedItems, 0, len(f.Items))

	for i := range f.Items {
		item := f.Items[i]

		link := item.Link
		if request.ItemLinkPrefix != "" {
			link = request.ItemLinkPrefix + link
		}

		desc := item.Description
		if desc == "" {
			desc = item.Content
		}

		rssItem := RSSFeedItem{
			ChannelURL:      f.Link,
			Title:           item.Title,
			Link:            link,
			Description:     stripHTML(desc),
			Categories:      item.Categories,
			HideCategories:  request.HideCategories,
			HideDescription: request.HideDescription,
		}

		if request.Title != "" {
			rssItem.ChannelName = request.Title
		} else {
			rssItem.ChannelName = f.Title
		}

		if item.Image != nil {
			rssItem.ImageURL = item.Image.URL
		} else if f.Image != nil {
			rssItem.ImageURL = f.Image.URL
		}

		if item.PublishedParsed != nil {
			rssItem.PublishedAt = *item.PublishedParsed
		} else {
			rssItem.PublishedAt = time.Now()
		}

		items = append(items, rssItem)
	}

	// Apply individual feed limits if configured
	if request.Limit != nil && *request.Limit > 0 {
		lim := *request.Limit
		if len(items) > lim {
			items = items[:lim]
		}
	}

	return items, nil
}

// GetItemsFromRSSFeeds fetches articles concurrently from multiple RSS feeds.
func GetItemsFromRSSFeeds(requests []RSSFeedRequest, preserveOrder bool) (RSSFeedItems, error) {
	job := newJob(getItemsFromRSSFeedTask, requests).withWorkers(10)
	feeds, errs, err := workerPoolDo(job)

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoContent, err)
	}

	failed := 0
	entries := make(RSSFeedItems, 0, len(feeds)*10)

	for i := range feeds {
		if errs[i] != nil {
			failed++
			slog.Error("failed to get rss feed", "error", errs[i], "url", requests[i].Url)
			continue
		}

		entries = append(entries, feeds[i]...)
	}

	if len(entries) == 0 {
		return nil, ErrNoContent
	}

	// Sort by publication date unless preserve-order is explicitly enabled.
	if !preserveOrder {
		entries.SortByNewest()
	}

	if failed > 0 {
		return entries, fmt.Errorf("%w: missing %d RSS feeds", ErrPartialContent, failed)
	}

	return entries, nil
}
