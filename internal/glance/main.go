package glance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/glanceapp/glance/internal/widget"
)

const defaultConfigYml = `pages:
  - title: Home
    columns:
      - size: full
        widgets:
          - type: clock
          - type: rss
            feeds:
              - url: https://news.ycombinator.com/rss
                title: Hacker News
`

func Main() int {
	options, err := ParseCliOptions()

	if err != nil {
		fmt.Println(err)
		return 1
	}

	configFile, err := os.Open(options.ConfigPath)

	if err != nil && os.IsNotExist(err) {
		if err := os.WriteFile(options.ConfigPath, []byte(defaultConfigYml), 0644); err != nil {
			fmt.Printf("failed creating default config file: %v\n", err)
			return 1
		}
		fmt.Printf("created default config file: %s\n", options.ConfigPath)
		configFile, err = os.Open(options.ConfigPath)
	}

	if err != nil {
		fmt.Printf("failed opening config file: %v\n", err)
		return 1
	}

	config, err := NewConfigFromYml(configFile)
	configFile.Close()

	if err != nil {
		fmt.Printf("failed parsing config file: %v\n", err)
		return 1
	}

	widget.GlobalTimezone = config.Server.Timezone

	if options.Intent == CliIntentServe {
		if err := initDB(options.ConfigPath); err != nil {
			fmt.Printf("failed to initialize database: %v\n", err)
			return 1
		}

		InitGoogle(config.Google.ClientID, config.Google.ClientSecret, config.Google.RedirectURL)

		app, err := NewApplication(config, options.ConfigPath)

		if err != nil {
			fmt.Printf("failed creating application: %v\n", err)
			return 1
		}

		if err := app.Serve(); err != nil {
			fmt.Printf("http server error: %v\n", err)
			return 1
		}
	}

	return 0
}

func configDir(configPath string) string {
	return filepath.Dir(configPath)
}

type glanceServiceProvider struct{}

func (p *glanceServiceProvider) SpotifyAuthorized() bool {
	if Store == nil {
		return false
	}
	auth, _ := Store.GetSetting("spotify_authorized", "false")
	return auth == "true"
}

func (p *glanceServiceProvider) GoogleAuthorized() bool {
	if Store == nil {
		return false
	}
	auth, _ := Store.GetSetting("google_authorized", "false")
	return auth == "true"
}

func (p *glanceServiceProvider) FetchGoogleEvents(ctx context.Context, calendarIDs []string, maxDaysAhead int) ([]widget.GoogleCalendarEvent, error) {
	return fetchGoogleEventsFromAPI(ctx, calendarIDs, maxDaysAhead)
}
