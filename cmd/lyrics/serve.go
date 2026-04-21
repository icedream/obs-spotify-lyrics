package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"

	cli "github.com/urfave/cli/v3"

	"github.com/icedream/spotify-lyrics-widget/internal/api"
)

func serve(ctx context.Context, c *cli.Command) error {
	client, err := buildClient(c)
	if err != nil {
		return err
	}

	addr := c.String(flagAddr)
	a, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return fmt.Errorf("parsing TCP address: %w", err)
	}

	srv := api.NewServer(client)

	mux := http.NewServeMux()
	mux.Handle("/ws", srv.Handler(ctx))

	httpSrv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	l, err := net.ListenTCP("tcp", a)
	if err != nil {
		return fmt.Errorf("setting up TCP listener: %w", err)
	}

	log.Printf("Lyrics server listening on http://%s/", l.Addr())

	if err := httpSrv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
