package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	dealerWSURL      = "wss://dealer.spotify.com/"
	connectStateBase = "https://spclient.wg.spotify.com/connect-state/v1/devices/hobs_"
)

// Artist represents a Spotify artist.
type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Album represents a Spotify album.
type Album struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Track represents a Spotify track.
type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []Artist `json:"artists"`
	Album      Album    `json:"album"`
	DurationMs int64    `json:"duration_ms"`
}

// CurrentlyPlayingResponse holds the current player state.
type CurrentlyPlayingResponse struct {
	IsPlaying            bool   `json:"is_playing"`
	ProgressMs           int64  `json:"progress_ms"`
	CurrentlyPlayingType string `json:"currently_playing_type"`
	// Item is nil when nothing is playing or the type is not "track".
	Item *Track `json:"item"`
}

// Internal connect-state response types.

type connectStateMeta map[string]string

type connectStateTrack struct {
	URI      string           `json:"uri"`
	Metadata connectStateMeta `json:"metadata"`
}

type connectStatePlayerState struct {
	Track                 connectStateTrack `json:"track"`
	IsPlaying             bool              `json:"is_playing"`
	IsPaused              bool              `json:"is_paused"`
	PositionAsOfTimestamp string            `json:"position_as_of_timestamp"`
	Timestamp             string            `json:"timestamp"`
}

type connectStateResp struct {
	PlayerState       connectStatePlayerState `json:"player_state"`
	ServerTimestampMs string                  `json:"server_timestamp_ms"`
}

// getDealerConnectionID opens a WebSocket to the Spotify dealer, reads until it
// receives the Spotify-Connection-Id, then closes the connection.
func (c *Client) getDealerConnectionID(accessToken string) (string, error) {
	conn, _, err := websocket.DefaultDialer.Dial(dealerWSURL+"?access_token="+accessToken, nil)
	if err != nil {
		return "", fmt.Errorf("dealer WebSocket dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	for i := 0; i < 5; i++ {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("dealer WebSocket read failed: %w", err)
		}
		var envelope struct {
			Headers map[string]string `json:"headers"`
		}
		if json.Unmarshal(msg, &envelope) == nil {
			if id := envelope.Headers["Spotify-Connection-Id"]; id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("dealer WebSocket did not provide a connection ID")
}

// CurrentlyPlaying returns information about the track currently playing on the
// user's Spotify account. Returns nil with no error when nothing is playing.
func (c *Client) CurrentlyPlaying(ctx context.Context) (*CurrentlyPlayingResponse, error) {
	accessToken, err := c.ensureToken()
	if err != nil {
		return nil, err
	}

	connID, err := c.getDealerConnectionID(accessToken)
	if err != nil {
		return nil, err
	}

	body := map[string]any{
		"member_type": "CONNECT_STATE",
		"device": map[string]any{
			"device_info": map[string]any{
				"capabilities": map[string]any{
					"can_be_player":           false,
					"hidden":                  true,
					"needs_full_player_state": true,
				},
			},
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal connect-state request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, connectStateBase+c.deviceID, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create connect-state request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Spotify-Connection-Id", connID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connect-state request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, &Error{Message: "unauthorized: sp_dc cookie may be expired", StatusCode: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		return nil, newError(resp)
	}

	var cs connectStateResp
	if err := json.NewDecoder(resp.Body).Decode(&cs); err != nil {
		return nil, fmt.Errorf("failed to decode connect-state response: %w", err)
	}

	uri := cs.PlayerState.Track.URI
	if !strings.HasPrefix(uri, "spotify:track:") {
		// Nothing playing or non-track content.
		return nil, nil
	}
	trackID := strings.TrimPrefix(uri, "spotify:track:")

	meta := cs.PlayerState.Track.Metadata
	durMs, _ := strconv.ParseInt(meta["duration"], 10, 64)

	// Artists: keys are "artist_name", "artist_name:1", "artist_name:2", ...
	// IDs come from "artist_uri", "artist_uri:1", ...
	var artists []Artist
	for i := 0; ; i++ {
		nameKey, uriKey := "artist_name", "artist_uri"
		if i > 0 {
			nameKey = fmt.Sprintf("artist_name:%d", i)
			uriKey = fmt.Sprintf("artist_uri:%d", i)
		}
		name, ok := meta[nameKey]
		if !ok {
			break
		}
		artists = append(artists, Artist{
			ID:   strings.TrimPrefix(meta[uriKey], "spotify:artist:"),
			Name: name,
		})
	}

	albumID := strings.TrimPrefix(meta["album_uri"], "spotify:album:")

	track := &Track{
		ID:         trackID,
		Name:       meta["title"],
		Artists:    artists,
		Album:      Album{ID: albumID, Name: meta["album_title"]},
		DurationMs: durMs,
	}

	// Compute progress: position at last timestamp + elapsed time if playing.
	var progressMs int64
	posMs, posErr := strconv.ParseInt(cs.PlayerState.PositionAsOfTimestamp, 10, 64)
	tsMs, tsErr := strconv.ParseInt(cs.PlayerState.Timestamp, 10, 64)
	if posErr == nil && tsErr == nil {
		progressMs = posMs
		if cs.PlayerState.IsPlaying && !cs.PlayerState.IsPaused {
			progressMs += time.Now().UnixMilli() - tsMs
		}
	}

	return &CurrentlyPlayingResponse{
		IsPlaying:            cs.PlayerState.IsPlaying && !cs.PlayerState.IsPaused,
		ProgressMs:           progressMs,
		CurrentlyPlayingType: "track",
		Item:                 track,
	}, nil
}
