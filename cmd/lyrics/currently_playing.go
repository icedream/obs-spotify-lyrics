package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/icedream/spotify-lyrics-widget/internal/spotify"
	cli "github.com/urfave/cli/v3"
)

func fetchCurrentlyPlaying(ctx context.Context, c *cli.Command) error {
	s, err := buildClient(c)
	if err != nil {
		return err
	}

np:
	log.Println("Fetching currently playing...")
	r, err := s.CurrentlyPlaying(ctx)
	if err != nil {
		var e *spotify.Error
		if errors.As(err, &e) && !e.RetryAfter.IsZero() {
			d := time.Until(e.RetryAfter)
			log.Println("Waiting", d)
			<-time.After(d)
			time.Sleep(time.Second * 3)
			goto np
		}
		return fmt.Errorf("fetching currently playing: %w", err)
	}

	spew.Dump(r)
	return nil
}
