package api

import (
	"encoding/json"

	"github.com/icedream/obs-spotify-lyrics/internal/spotify"
)

// msg is the JSON payload sent over the WebSocket to every connected widget.
type msg struct {
	Type       string         `json:"type"` // "playing" or "idle"
	Track      *spotify.Track `json:"track,omitempty"`
	Lyrics     []lyricLine    `json:"lyrics,omitempty"`
	PositionMs int64          `json:"position_ms,omitempty"`
	IsPlaying  bool           `json:"is_playing,omitempty"`
}

// lyricLine is a single lyric line with its start timestamp in milliseconds.
type lyricLine struct {
	StartMs int64  `json:"start_ms"`
	EndMs   int64  `json:"end_ms,omitempty"`
	Words   string `json:"words"`
}

var idleJSON []byte

func init() {
	idleJSON, _ = json.Marshal(msg{Type: "idle"})
}
