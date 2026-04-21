package spotify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
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

// GetLRCLyrics converts a slice of LyricsLines to LRC format.
func GetLRCLyrics(lines []LyricsLine) []LRCLine {
	lrc := make([]LRCLine, 0, len(lines))
	for _, line := range lines {
		ms, _ := strconv.ParseInt(line.StartTimeMs, 10, 64)
		lrc = append(lrc, LRCLine{
			TimeTag: FormatMS(ms),
			Words:   line.Words,
		})
	}
	return lrc
}

// GetSRTLyrics converts a slice of LyricsLines to SRT format.
// The last line is omitted as its end time is unknown.
func GetSRTLyrics(lines []LyricsLine) []SRTLine {
	if len(lines) == 0 {
		return nil
	}
	srt := make([]SRTLine, 0, len(lines)-1)
	for i := 1; i < len(lines); i++ {
		startMs, _ := strconv.ParseInt(lines[i-1].StartTimeMs, 10, 64)
		endMs, _ := strconv.ParseInt(lines[i].StartTimeMs, 10, 64)
		srt = append(srt, SRTLine{
			Index:     i,
			StartTime: FormatSRT(startMs),
			EndTime:   FormatSRT(endMs),
			Words:     lines[i-1].Words,
		})
	}
	return srt
}

// GetRawLyrics returns lyrics as a plain newline-delimited string.
func GetRawLyrics(lines []LyricsLine) string {
	var sb strings.Builder
	for _, line := range lines {
		sb.WriteString(line.Words)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// FormatMS formats a duration in milliseconds as mm:ss.cc for use in LRC files.
func FormatMS(ms int64) string {
	thSecs := ms / 1000
	centiseconds := (ms % 1000) / 10
	return fmt.Sprintf("%02d:%02d.%02d", thSecs/60, thSecs%60, centiseconds)
}

// FormatSRT formats a duration in milliseconds as hh:mm:ss,ms for use in SRT files.
func FormatSRT(ms int64) string {
	hours := ms / 3_600_000
	minutes := (ms % 3_600_000) / 60_000
	seconds := (ms % 60_000) / 1_000
	millis := ms % 1_000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, millis)
}
