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

func fetchLyrics(ctx context.Context, c *cli.Command) error {
	s, err := buildClient(c)
	if err != nil {
		return err
	}

	trackID := c.StringArg(argTrackID)
	if len(trackID) == 0 {
		return errors.New("track ID must not be empty")
	}

lyrics:
	logger.Debugf("Fetching lyrics: %s", trackID)
	r, err := s.Lyrics(ctx, trackID)
	if err != nil {
		var e *spotify.Error
		if errors.As(err, &e) && !e.RetryAfter.IsZero() {
			d := time.Until(e.RetryAfter)
			logger.Debugf("Waiting %v", d)
			<-time.After(d)
			time.Sleep(time.Second * 3)
			goto lyrics
		}
		return fmt.Errorf("fetching lyrics: %w", err)
	}

	spew.Dump(r)
	return nil
}
