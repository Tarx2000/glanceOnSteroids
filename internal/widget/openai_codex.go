package widget

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/glanceapp/glance/internal/assets"
	"github.com/glanceapp/glance/internal/feed"
)

func init() {
	Register("openai-codex", func() Widget { return &OpenAICodex{} })
}

// OpenAICodex tracks subscription rate limits, quotas, and reset timers for OpenAI Codex.
type OpenAICodex struct {
	widgetBase       `yaml:",inline"`
	Token            OptionalEnvString `yaml:"token"`
	AuthFile         string            `yaml:"auth-file"`
	AccountID        OptionalEnvString `yaml:"account-id"`
	Endpoint         string            `yaml:"endpoint"`
	ShowSessionLimit *BoolField        `yaml:"show-session-limit"`
	ShowWeeklyLimit  *BoolField        `yaml:"show-weekly-limit"`
	ShowCredits      *BoolField        `yaml:"show-credits"`

	// Runtime fields for template rendering (yaml:"-")
	PlanType               string  `yaml:"-"`
	PlanBadgeClass         string  `yaml:"-"`
	IsRateLimited          bool    `yaml:"-"`
	StatusText             string  `yaml:"-"`
	StatusClass            string  `yaml:"-"`
	HasPrimary             bool    `yaml:"-"`
	PrimaryUsedPct         float64 `yaml:"-"`
	PrimaryLeftPct         float64 `yaml:"-"`
	PrimaryProgressClass   string  `yaml:"-"`
	PrimaryResetDesc       string  `yaml:"-"`
	PrimaryWindowLabel     string  `yaml:"-"`
	HasSecondary           bool    `yaml:"-"`
	SecondaryUsedPct       float64 `yaml:"-"`
	SecondaryLeftPct       float64 `yaml:"-"`
	SecondaryProgressClass string  `yaml:"-"`
	SecondaryResetDesc     string  `yaml:"-"`
	SecondaryWindowLabel   string  `yaml:"-"`
	HasCredits             bool    `yaml:"-"`
	CreditBalance          string  `yaml:"-"`
	IsCreditsUnlimited     bool    `yaml:"-"`
	DisplaySessionLimit    bool    `yaml:"-"`
	DisplayWeeklyLimit     bool    `yaml:"-"`
	DisplayCredits         bool    `yaml:"-"`
}

func (widget *OpenAICodex) Initialize() error {
	widget.Type = "openai-codex"
	widget.withTitle("OpenAI Codex Quota")
	widget.withCacheDuration(5 * time.Minute)
	return nil
}

func formatCodexCountdown(resetAfterSeconds int64, resetAt int64) string {
	secs := resetAfterSeconds
	if secs <= 0 && resetAt > 0 {
		secs = resetAt - time.Now().Unix()
	}
	if secs <= 0 {
		return "Resets soon"
	}

	d := time.Duration(secs) * time.Second
	if d < time.Minute {
		return "< 1m left"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm left", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		mins := int(d.Minutes()) % 60
		if mins > 0 {
			return fmt.Sprintf("%dh %dm left", hours, mins)
		}
		return fmt.Sprintf("%dh left", hours)
	}

	days := int(d.Hours()) / 24
	remHours := int(d.Hours()) % 24
	if remHours > 0 {
		return fmt.Sprintf("%dd %dh left", days, remHours)
	}
	return fmt.Sprintf("%dd left", days)
}

func formatWindowLabel(windowSeconds int64, defaultLabel string) string {
	if windowSeconds <= 0 {
		return defaultLabel
	}
	hours := windowSeconds / 3600
	if hours <= 1 {
		return "1-Hour Window"
	}
	if hours < 24 {
		return fmt.Sprintf("%d-Hour Window", hours)
	}
	days := hours / 24
	if days == 7 {
		return "Weekly Limit (7d)"
	}
	return fmt.Sprintf("%d-Day Window", days)
}

func getProgressClass(usedPct float64) string {
	if usedPct >= 100 {
		return "codex-progress-danger"
	}
	if usedPct >= 80 {
		return "codex-progress-warning"
	}
	return "codex-progress-normal"
}

func (widget *OpenAICodex) Update(ctx context.Context, services ExternalServiceProvider) {
	token := strings.TrimSpace(string(widget.Token))
	accountID := strings.TrimSpace(string(widget.AccountID))

	// Resolve credentials from auth.json if direct token is empty
	if token == "" {
		authPath := widget.AuthFile
		if authPath == "" {
			authPath = "~/.codex/auth.json"
		}
		tok, acc, err := feed.ReadCodexAuthFile(authPath)
		if err != nil {
			widget.canContinueUpdateAfterHandlingErr(fmt.Errorf("OpenAI Codex authentication error: %w", err))
			return
		}
		token = tok
		if accountID == "" {
			accountID = acc
		}
	}

	endpoint := widget.Endpoint
	if endpoint == "" {
		endpoint = feed.DefaultCodexEndpoint
	}

	resp, err := feed.FetchCodexUsage(ctx, feed.CodexHTTPClient, endpoint, token, accountID)
	if err != nil {
		widget.canContinueUpdateAfterHandlingErr(err)
		return
	}

	// Calculate display values
	plan := strings.TrimSpace(resp.PlanType)
	if plan == "" {
		plan = "standard"
	}
	planBadgeClass := "codex-badge-" + strings.ToLower(plan)

	var hasPrimary bool
	var primaryUsed, primaryLeft float64
	var primaryReset, primaryLabel, primaryClass string
	isRateLimited := false

	if resp.RateLimit != nil && resp.RateLimit.PrimaryWindow != nil {
		pw := resp.RateLimit.PrimaryWindow
		hasPrimary = true
		primaryUsed = pw.UsedPercent
		if primaryUsed > 100 {
			primaryUsed = 100
		} else if primaryUsed < 0 {
			primaryUsed = 0
		}
		primaryLeft = math.Max(0, 100-primaryUsed)
		primaryReset = formatCodexCountdown(pw.ResetAfterSeconds, pw.ResetAt)
		primaryLabel = formatWindowLabel(pw.LimitWindowSeconds, "Session Limit (5h)")
		primaryClass = getProgressClass(primaryUsed)
		if pw.UsedPercent >= 100 {
			isRateLimited = true
		}
	}

	var hasSecondary bool
	var secondaryUsed, secondaryLeft float64
	var secondaryReset, secondaryLabel, secondaryClass string

	if resp.RateLimit != nil && resp.RateLimit.SecondaryWindow != nil {
		sw := resp.RateLimit.SecondaryWindow
		hasSecondary = true
		secondaryUsed = sw.UsedPercent
		if secondaryUsed > 100 {
			secondaryUsed = 100
		} else if secondaryUsed < 0 {
			secondaryUsed = 0
		}
		secondaryLeft = math.Max(0, 100-secondaryUsed)
		secondaryReset = formatCodexCountdown(sw.ResetAfterSeconds, sw.ResetAt)
		secondaryLabel = formatWindowLabel(sw.LimitWindowSeconds, "Weekly Limit (7d)")
		secondaryClass = getProgressClass(secondaryUsed)
		if sw.UsedPercent >= 100 {
			isRateLimited = true
		}
	}

	var hasCredits bool
	var creditBalance string
	var isCreditsUnlimited bool
	if resp.Credits != nil {
		hasCredits = true
		isCreditsUnlimited = resp.Credits.Unlimited
		if isCreditsUnlimited {
			creditBalance = "Unlimited"
		} else if resp.Credits.Balance != nil {
			switch b := resp.Credits.Balance.(type) {
			case float64:
				creditBalance = fmt.Sprintf("$%.2f", b)
			case int:
				creditBalance = fmt.Sprintf("$%d.00", b)
			case string:
				if b != "" {
					creditBalance = b
				} else {
					creditBalance = "$0.00"
				}
			default:
				creditBalance = fmt.Sprintf("%v", b)
			}
		} else {
			creditBalance = "$0.00"
		}
	}

	statusText := "Healthy"
	statusClass := "codex-status-healthy"
	if isRateLimited {
		statusText = "Rate Limited"
		statusClass = "codex-status-limited"
	} else if (hasPrimary && primaryUsed >= 80) || (hasSecondary && secondaryUsed >= 80) {
		statusText = "Near Limit"
		statusClass = "codex-status-warning"
	}

	dispSession := widget.ShowSessionLimit == nil || *widget.ShowSessionLimit
	dispWeekly := widget.ShowWeeklyLimit == nil || *widget.ShowWeeklyLimit
	dispCredits := widget.ShowCredits == nil || *widget.ShowCredits

	widget.Lock()
	widget.PlanType = strings.ToUpper(plan)
	widget.PlanBadgeClass = planBadgeClass
	widget.IsRateLimited = isRateLimited
	widget.StatusText = statusText
	widget.StatusClass = statusClass
	widget.HasPrimary = hasPrimary
	widget.PrimaryUsedPct = primaryUsed
	widget.PrimaryLeftPct = primaryLeft
	widget.PrimaryProgressClass = primaryClass
	widget.PrimaryResetDesc = primaryReset
	widget.PrimaryWindowLabel = primaryLabel
	widget.HasSecondary = hasSecondary
	widget.SecondaryUsedPct = secondaryUsed
	widget.SecondaryLeftPct = secondaryLeft
	widget.SecondaryProgressClass = secondaryClass
	widget.SecondaryResetDesc = secondaryReset
	widget.SecondaryWindowLabel = secondaryLabel
	widget.HasCredits = hasCredits
	widget.CreditBalance = creditBalance
	widget.IsCreditsUnlimited = isCreditsUnlimited
	widget.DisplaySessionLimit = bool(dispSession)
	widget.DisplayWeeklyLimit = bool(dispWeekly)
	widget.DisplayCredits = bool(dispCredits)
	widget.Unlock()

	widget.canContinueUpdateAfterHandlingErr(nil)
}

func (widget *OpenAICodex) Render() template.HTML {
	return widget.render(widget, assets.OpenAICodexTemplate)
}
