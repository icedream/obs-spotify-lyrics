package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type tokenResponse struct {
	AccessToken                      string `json:"accessToken"`
	AccessTokenExpirationTimestampMs int64  `json:"accessTokenExpirationTimestampMs"`
	IsAnonymous                      bool   `json:"isAnonymous"`
}

type serverTimeResponse struct {
	ServerTime int64 `json:"serverTime"`
}

// LyricsLine represents a single line of lyrics with timing information.
type LyricsLine struct {
	StartTimeMs  string `json:"startTimeMs"`
	Words        string `json:"words"`
	SyllabicLine bool   `json:"syllabicLine"`
	EndTimeMs    string `json:"endTimeMs,omitempty"`
}

// Lyrics contains the lyrics data returned by Spotify.
type Lyrics struct {
	SyncType         string       `json:"syncType"`
	Lines            []LyricsLine `json:"lines"`
	Provider         string       `json:"provider"`
	ProviderLyricsID string       `json:"providerLyricsId"`
	Language         string       `json:"language"`
}

// LyricsResponse is the top-level response from the Spotify lyrics API.
type LyricsResponse struct {
	// Lyrics may be nil if Spotify returns a response without lyrics.
	Lyrics          *Lyrics         `json:"lyrics"`
	Colors          json.RawMessage `json:"colors"`
	HasVocalRemoval bool            `json:"hasVocalRemoval"`
}

// Error represents an error returned by the Spotify API, with an associated HTTP status code.
type Error struct {
	Message    string
	StatusCode int
	RetryAfter time.Time
}

func (e *Error) Error() string {
	return e.Message
}

func newError(r *http.Response) *Error {
	err := &Error{
		Message:    fmt.Sprintf("Spotify API error (HTTP %d)", r.StatusCode),
		StatusCode: r.StatusCode,
	}

	if ra := r.Header.Get("Retry-After"); len(ra) != 0 {
		err.Message += ", retry after " + ra
		seconds, dateErr := strconv.ParseInt(ra, 10, 32)
		if dateErr != nil {
			raTime, dateErr := http.ParseTime(ra)
			if dateErr != nil {
				return err
			}
			err.RetryAfter = raTime
		} else {
			err.RetryAfter = time.Now().Add(time.Duration(seconds) * time.Second)
		}
	}

	return err
}
