package api

import (
	"context"
	"net/http"

	"github.com/icedream/spotify-lyrics-widget/internal/spotify"
)

// Server is the OBS lyrics widget WebSocket API server.
type Server struct {
	client *spotify.Client
}

// NewServer creates a Server backed by the given Spotify client.
func NewServer(client *spotify.Client) *Server {
	return &Server{client: client}
}

// Handler starts the Spotify poller and returns an http.Handler that serves
// the WebSocket endpoint. The poller runs until ctx is cancelled.
func (s *Server) Handler(ctx context.Context) http.Handler {
	h := newHub()
	p := newPoller(s.client, h)
	h.mu.Lock()
	h.snapshotFn = p.snapshot
	h.mu.Unlock()
	go p.run(ctx)
	return h.wsHandler()
}
