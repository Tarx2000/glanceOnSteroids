package glance

import (
	"fmt"
	"os"

	"github.com/glanceapp/glance/internal/widget"
)

func Main() int {
	options, err := ParseCliOptions()

	if err != nil {
		fmt.Println(err)
		return 1
	}

	configFile, err := os.Open(options.ConfigPath)

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

	if options.Intent == CliIntentServe {
		// Initialize the SQLite settings database
		if err := initDB(options.ConfigPath); err != nil {
			fmt.Printf("failed to initialize database: %v\n", err)
			return 1
		}

		// Wire Spotify authorized callback check
		widget.SpotifyAuthorizedCheck = func() bool {
			auth, _ := dbGetSetting("spotify_authorized", "false")
			return auth == "true"
		}

		// Initialize WebSocket hub
		initWebSocket()

		// Initialize Spotify client credentials
		InitSpotify(config.Spotify.ClientID, config.Spotify.ClientSecret, config.Spotify.RedirectURL)

		// Start Spotify poller
		StartSpotifyPoller()

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
