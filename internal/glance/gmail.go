package glance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/glanceapp/glance/internal/widget"
)

func fetchGmailUnreadCountFromAPI(ctx context.Context) (int, []widget.GmailEmail, error) {
	token, err := getGoogleAccessToken()
	if err != nil {
		return 0, nil, err
	}

	// Fetch unread messages from the last 7 days
	query := "is:unread newer_than:7d"
	apiURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages?q=%s&maxResults=50", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := googleHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Error("[Gmail] List messages failed", "status", resp.StatusCode, "body", string(body))
		return 0, nil, fmt.Errorf("gmail api list error: %d", resp.StatusCode)
	}

	var listResp struct {
		Messages []struct {
			ID       string `json:"id"`
			ThreadID string `json:"threadId"`
		} `json:"messages"`
		ResultSizeEstimate int `json:"resultSizeEstimate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return 0, nil, err
	}

	totalUnread := len(listResp.Messages)
	if totalUnread == 0 && listResp.ResultSizeEstimate > 0 {
		totalUnread = listResp.ResultSizeEstimate
	}

	var emails []widget.GmailEmail
	limit := 5
	if len(listResp.Messages) < limit {
		limit = len(listResp.Messages)
	}

	for i := 0; i < limit; i++ {
		msgID := listResp.Messages[i].ID
		detailURL := fmt.Sprintf("https://gmail.googleapis.com/gmail/v1/users/me/messages/%s?format=metadata&metadataHeaders=Subject&metadataHeaders=From&metadataHeaders=Date", msgID)

		dReq, err := http.NewRequestWithContext(ctx, "GET", detailURL, nil)
		if err != nil {
			continue
		}
		dReq.Header.Set("Authorization", "Bearer "+token)

		dResp, err := googleHTTPClient.Do(dReq)
		if err != nil {
			continue
		}
		defer dResp.Body.Close()

		if dResp.StatusCode != http.StatusOK {
			continue
		}

		var detail struct {
			Payload struct {
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"payload"`
		}

		if json.NewDecoder(dResp.Body).Decode(&detail) == nil {
			var subject, sender, date string
			for _, h := range detail.Payload.Headers {
				switch strings.ToLower(h.Name) {
				case "subject":
					subject = h.Value
				case "from":
					sender = h.Value
				case "date":
					date = h.Value
				}
			}
			emails = append(emails, widget.GmailEmail{
				Subject: subject,
				Sender:  sender,
				Date:    date,
			})
		}
	}

	return totalUnread, emails, nil
}
