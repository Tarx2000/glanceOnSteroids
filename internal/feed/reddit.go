package feed

import (
	"context"
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

type subredditResponseJson struct {
	Data struct {
		Children []struct {
			Data struct {
				Id            string  `json:"id"`
				Title         string  `json:"title"`
				Upvotes       int     `json:"ups"`
				Url           string  `json:"url"`
				Time          float64 `json:"created"`
				CommentsCount int     `json:"num_comments"`
				Domain        string  `json:"domain"`
				Permalink     string  `json:"permalink"`
				Stickied      bool    `json:"stickied"`
				Pinned        bool    `json:"pinned"`
				IsSelf        bool    `json:"is_self"`
				Thumbnail     string  `json:"thumbnail"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func FetchSubredditPosts(subreddit, sort, topPeriod, search, commentsUrlTemplate, requestUrlTemplate string) (ForumPosts, error) {
	query := url.Values{}
	var requestUrl string

	if search != "" {
		query.Set("q", search+" subreddit:"+subreddit)
		query.Set("sort", sort)
	}

	if sort == "top" {
		query.Set("t", topPeriod)
	}

	if search != "" {
		requestUrl = fmt.Sprintf("https://www.reddit.com/search.json?%s", query.Encode())
	} else {
		requestUrl = fmt.Sprintf("https://www.reddit.com/r/%s/%s.json?%s", subreddit, sort, query.Encode())
	}

	if requestUrlTemplate != "" {
		requestUrl = strings.ReplaceAll(requestUrlTemplate, "{REQUEST-URL}", requestUrl)
	}

	request, err := http.NewRequest("GET", requestUrl, nil)

	if err != nil {
		return nil, err
	}

	// Required to increase rate limit, otherwise Reddit randomly returns 429 even after just 2 requests
	addBrowserUserAgentHeader(request)

	// Fetch or use cached loid cookie to bypass API restrictions
	loid, err := getRedditLoidCookie()
	if err == nil && loid != "" {
		request.AddCookie(&http.Cookie{Name: "loid", Value: loid})
	}

	responseJson, err := decodeJsonFromRequest[subredditResponseJson](redditHTTPClient, request)

	if err != nil {
		return nil, err
	}

	if len(responseJson.Data.Children) == 0 {
		return nil, fmt.Errorf("no posts found")
	}

	posts := make(ForumPosts, 0, len(responseJson.Data.Children))

	for i := range responseJson.Data.Children {
		post := &responseJson.Data.Children[i].Data

		if post.Stickied || post.Pinned {
			continue
		}

		var commentsUrl string

		if commentsUrlTemplate == "" {
			commentsUrl = "https://www.reddit.com" + post.Permalink
		} else {
			commentsUrl = strings.ReplaceAll(commentsUrlTemplate, "{SUBREDDIT}", subreddit)
			commentsUrl = strings.ReplaceAll(commentsUrl, "{POST-ID}", post.Id)
			commentsUrl = strings.ReplaceAll(commentsUrl, "{POST-PATH}", strings.TrimLeft(post.Permalink, "/"))
		}

		forumPost := ForumPost{
			Title:           html.UnescapeString(post.Title),
			DiscussionUrl:   commentsUrl,
			TargetUrlDomain: post.Domain,
			CommentCount:    post.CommentsCount,
			Score:           post.Upvotes,
			TimePosted:      time.Unix(int64(post.Time), 0),
		}

		if post.Thumbnail != "" && post.Thumbnail != "self" && post.Thumbnail != "default" {
			forumPost.ThumbnailUrl = post.Thumbnail
		}

		if !post.IsSelf {
			forumPost.TargetUrl = post.Url
		}

		posts = append(posts, forumPost)
	}

	return posts, nil
}

// ----------------------------------------------------
// Reddit Cloudflare/TLS Fingerprint Bypass Helpers
// ----------------------------------------------------

var (
	redditChallengePattern = regexp.MustCompile(`await\(async \w+\s*=>\s*\w+\s*\+\s*\w+\)\("([^"]+)"\)`)
	redditTokenPattern     = regexp.MustCompile(`name="token"\s+value="([^"]+)"`)
)

// redditHTTPClient uses uTLS to mimic a real Firefox browser handshake.
// This prevents Cloudflare/Reddit TLS handshake detection from returning 403 Forbidden.
var redditHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			tcpConn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			uconn := utls.UClient(tcpConn, &utls.Config{
				ServerName: host,
			}, utls.HelloFirefox_Auto)

			if err := uconn.HandshakeContext(ctx); err != nil {
				tcpConn.Close()
				return nil, err
			}

			return uconn, nil
		},
	},
}

// getRedditLoidCookie resolves and caches a single 'loid' cookie for 6 hours
// to avoid hitting the challenge page too frequently.
var getRedditLoidCookie = func() func() (string, error) {
	var lastUpdate time.Time
	var cachedLoid string
	var mu sync.Mutex

	return func() (string, error) {
		mu.Lock()
		defer mu.Unlock()

		if time.Since(lastUpdate) < 6*time.Hour && cachedLoid != "" {
			return cachedLoid, nil
		}

		loid, err := fetchRedditLoidCookie()
		if err != nil {
			if cachedLoid != "" {
				fmt.Printf("[Reddit] Challenge solver error, falling back to cached cookie: %v\n", err)
				return cachedLoid, nil
			}
			return "", err
		}

		lastUpdate = time.Now()
		cachedLoid = loid
		return loid, nil
	}
}()

// fetchRedditLoidCookie gets the root reddit page, solves the basic JS challenge,
// and retrieves a 'loid' cookie from the response.
func fetchRedditLoidCookie() (string, error) {
	request, err := http.NewRequest("GET", "https://www.reddit.com/", nil)
	if err != nil {
		return "", err
	}
	addBrowserUserAgentHeader(request)

	response, err := redditHTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d when requesting challenge page", response.StatusCode)
	}

	challengeBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	challengeMatches := redditChallengePattern.FindSubmatch(challengeBody)
	tokenMatches := redditTokenPattern.FindSubmatch(challengeBody)

	if challengeMatches == nil {
		return "", fmt.Errorf("no JS challenge found")
	}
	if tokenMatches == nil {
		return "", fmt.Errorf("no token found in challenge page")
	}

	challengeStr := string(challengeMatches[1])
	token := string(tokenMatches[1])
	solution := challengeStr + challengeStr

	params := url.Values{
		"solution":     {solution},
		"js_challenge": {"1"},
		"token":        {token},
	}

	request, err = http.NewRequest("GET", "https://www.reddit.com/?"+params.Encode(), nil)
	if err != nil {
		return "", err
	}
	addBrowserUserAgentHeader(request)

	response, err = redditHTTPClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d when submitting challenge solution", response.StatusCode)
	}

	for _, cookie := range response.Cookies() {
		if cookie.Name == "loid" {
			return cookie.Value, nil
		}
	}

	return "", fmt.Errorf("no loid cookie found in headers")
}
