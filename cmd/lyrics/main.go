package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/icedream/spotify-lyrics-widget/internal/browser"
	"github.com/icedream/spotify-lyrics-widget/internal/logger"
	"github.com/icedream/spotify-lyrics-widget/internal/spotify"
	"github.com/kirsle/configdir"
	"github.com/stoewer/go-strcase"
	altsrc "github.com/urfave/cli-altsrc/v3"
	yamlsrc "github.com/urfave/cli-altsrc/v3/yaml"
	cli "github.com/urfave/cli/v3"
)

const appName = "spotify-lyrics"

// version is set at build time via -ldflags "-X main.version=…".
var version = "dev"

const (
	flagSpotifyCookie   = "spotify-cookie"
	flagSpotifyDeviceID = "spotify-device-id"
	flagAddr            = "addr"

	argTrackID = "track-id"
)

var configPath string

func init() {
	configPath = configdir.LocalConfig(appName)
}

func getFlagSources(flagName string) cli.ValueSourceChain {
	return cli.NewValueSourceChain(
		yamlsrc.YAML(
			strcase.SnakeCase(flagName),
			altsrc.StringSourcer(filepath.Join(".", "config.yml"))),
		yamlsrc.YAML(
			strcase.SnakeCase(flagName),
			altsrc.StringSourcer(filepath.Join(configPath, "config.yml"))),
		cli.EnvVar(strcase.UpperSnakeCase(flagSpotifyCookie)),
	)
}

func buildClient(c *cli.Command) (*spotify.Client, error) {
	spDC := c.String(flagSpotifyCookie)
	if len(spDC) == 0 {
		logger.Info("No sp_dc cookie configured, searching installed browsers...")
		var err error
		spDC, err = browser.FindCookie("sp_dc", ".spotify.com")
		if err != nil {
			return nil, fmt.Errorf("sp_dc cookie not configured and auto-discovery failed: %w", err)
		}
		logger.Info("Found sp_dc cookie in browser.")
	}

	deviceID := c.String(flagSpotifyDeviceID)

	return spotify.NewClient(spDC, deviceID), nil
}

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	logger.Infof("Spotify lyrics server %s", version)
	var (
		flags = []cli.Flag{
			&cli.StringFlag{
				Name:    flagSpotifyCookie,
				Usage:   "sp_dc cookie (auto-discovered from browsers if not set)",
				Sources: getFlagSources(flagSpotifyCookie),
			},
			&cli.StringFlag{
				Name:    flagSpotifyDeviceID,
				Usage:   "Spotify device ID (defaults to randomly generated)",
				Sources: getFlagSources(flagSpotifyDeviceID),
			},
		}

		c = &cli.Command{
			Name:           "lyrics",
			Copyright:      "2026 Carl Kittelberger",
			Version:        version,
			Flags:          flags,
			DefaultCommand: "serve",
			Commands: []*cli.Command{
				{
					Name:        "serve",
					Description: "Starts the OBS lyrics widget server",
					Flags: []cli.Flag{
						&cli.StringFlag{
							Name:    flagAddr,
							Usage:   "address to listen on",
							Value:   "localhost:8080",
							Sources: getFlagSources(flagAddr),
						},
					},
					Action: serve,
				},
				{
					Name:        "current",
					Description: "Displays the currently playing song metadata",
					Action:      fetchCurrentlyPlaying,
				},
				{
					Name:        "lyrics",
					Description: "Gets the lyrics for a specific song ID",
					Arguments: []cli.Argument{
						&cli.StringArg{
							Name: argTrackID,
						},
					},
					Action: fetchLyrics,
				},
			},
		}
	)

	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := c.Run(ctx, os.Args)
	if err != nil {
		logger.Errorf("Error: %v", err)
		exitCode = 1
	}

	return
}
