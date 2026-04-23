package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/icedream/obs-spotify-lyrics/internal/logger"
	"github.com/icedream/obs-spotify-lyrics/internal/spotify"
	cli "github.com/urfave/cli/v3"
)

func fetchCurrentlyPlaying(ctx context.Context, c *cli.Command) error {
	s, err := buildClient(c)
	if err != nil {
		return err
	}

np:
	logger.Debug("Fetching currently playing...")
	r, err := s.CurrentlyPlaying(ctx)
	if err != nil {
		var e *spotify.Error
		if errors.As(err, &e) && !e.RetryAfter.IsZero() {
			d := time.Until(e.RetryAfter)
			logger.Debugf("Waiting %v", d)
			<-time.After(d)
			time.Sleep(time.Second * 3)
			goto np
		}
		return fmt.Errorf("fetching currently playing: %w", err)
	}

	spew.Dump(r)
	return nil
}
