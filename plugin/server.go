package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/icedream/spotify-lyrics-widget/internal/api"
	"github.com/icedream/spotify-lyrics-widget/internal/browser"
	"github.com/icedream/spotify-lyrics-widget/internal/logger"
	"github.com/icedream/spotify-lyrics-widget/internal/spotify"
	"github.com/icedream/spotify-lyrics-widget/internal/widget"
)

var (
	srvMu           sync.Mutex
	srvCancel       context.CancelFunc
	serverLastError string

	// widgetBaseURL is the URL other parts of the plugin read to populate
	// the nested browser_source. Updated by serverStart/serverStop.
	widgetBaseURL string
)

// serverStart launches (or relaunches) the embedded HTTP+WebSocket server.
// spDC and deviceID may be empty to trigger auto-discovery / random generation.
// On success it sets widgetBaseURL; on failure widgetBaseURL stays empty.
// Returns an error if the server could not be started.
func serverStart(port int, spDC, deviceID string) error {
	// Resolve sp_dc before taking the lock: browser.FindCookie may block on
	// the system keychain and we must not stall status reads while it does.
	if spDC == "" {
		logger.Info("no sp_dc configured, searching installed browsers...")
		var err error
		spDC, err = browser.FindCookie("sp_dc", ".spotify.com")
		if err != nil {
			logger.Warnf("sp_dc auto-discovery failed: %v", err)
			srvMu.Lock()
			if errors.Is(err, browser.ErrBrowserLocked) {
				serverLastError = "sp_dc auto-discovery failed: a browser profile was locked - close running browsers and try again, or enter the cookie manually"
			} else {
				serverLastError = "sp_dc cookie not found, please enter it in the plugin settings"
			}
			srvMu.Unlock()
			return err
		}
		logger.Info("found sp_dc cookie in browser")
	}

	srvMu.Lock()
	defer srvMu.Unlock()

	if srvCancel != nil {
		srvCancel()
		srvCancel = nil
		widgetBaseURL = ""
	}

	serverLastError = ""

	addr := fmt.Sprintf("localhost:%d", port)
	client := spotify.NewClient(spDC, deviceID)
	srv := api.NewServer(client)

	ctx, cancel := context.WithCancel(context.Background())

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

	l, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Errorf("could not bind %s: %v", addr, err)
		serverLastError = fmt.Sprintf("could not bind %s: %v", addr, err)
		cancel()
		return err
	}

	srvCancel = cancel
	widgetBaseURL = "http://" + l.Addr().String()
	logger.Infof("server running on %s/", widgetBaseURL)

	go func() {
		if err := httpSrv.Serve(l); !errors.Is(err, http.ErrServerClosed) && err != nil {
			logger.Errorf("server error: %v", err)
		}
	}()

	return nil
}

// serverStop shuts down the running server if any.
func serverStop() {
	srvMu.Lock()
	defer srvMu.Unlock()
	if srvCancel != nil {
		srvCancel()
		srvCancel = nil
		widgetBaseURL = ""
	}
}

// currentWidgetURL returns a copy of widgetBaseURL under the lock.
func currentWidgetURL() string {
	srvMu.Lock()
	defer srvMu.Unlock()
	return widgetBaseURL
}
