package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	cli "github.com/urfave/cli/v3"

	"github.com/icedream/obs-spotify-lyrics/internal/api"
	"github.com/icedream/obs-spotify-lyrics/internal/logger"
	"github.com/icedream/obs-spotify-lyrics/internal/widget"
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
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(widget.HTML)
	})
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

	logger.Infof("Lyrics server listening on ws://%s/ws", l.Addr())
	logger.Infof("HTML widget available at http://%s/", l.Addr())

	if err := httpSrv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
