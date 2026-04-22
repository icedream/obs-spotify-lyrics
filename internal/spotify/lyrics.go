package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const lyricsURL = "https://spclient.wg.spotify.com/color-lyrics/v2/track/"

// Lyrics retrieves the lyrics for a Spotify track by its ID.
// It returns an error of type *Error for API-level failures (e.g. 404, 429).
func (c *Client) Lyrics(ctx context.Context, trackID string) (*LyricsResponse, error) {
	token, err := c.ensureToken()
	if err != nil {
		return nil, err
	}

	if strings.Contains(trackID, "?") {
		return nil, errors.New("unsupported track ID")
	}

	reqURL := lyricsURL + trackID + "?format=json&market=from_token"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create lyrics request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.0.0 Safari/537.36")
	req.Header.Set("App-platform", "WebPlayer")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lyrics request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return nil, &Error{Message: "rate limited by Spotify, please try again later", StatusCode: resp.StatusCode}
	case http.StatusNotFound:
		return nil, &Error{Message: "lyrics not found for this track", StatusCode: resp.StatusCode}
	}
	if resp.StatusCode >= 400 {
		return nil, newError(resp)
	}

	var lyricsResp LyricsResponse
	if err := json.NewDecoder(resp.Body).Decode(&lyricsResp); err != nil {
		return nil, fmt.Errorf("decoding lyrics response: %w", err)
	}
	if lyricsResp.Lyrics == nil {
		return nil, fmt.Errorf("no lyrics in response")
	}

	return &lyricsResp, nil
}
