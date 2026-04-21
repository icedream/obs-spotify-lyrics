package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/icedream/spotify-lyrics-widget/internal/spotify"
)

const (
	// How often to poll Spotify while a track is playing/while idle.
	pollIntervalPlaying = 3 * time.Second
	pollIntervalIdle    = 5 * time.Second

	// If the reported position differs from the extrapolated position by more
	// than this, we treat it as a seek and push an immediate update.
	seekThresholdMs = int64(1500)
)

// poller polls Spotify periodically and broadcasts state changes to the hub.
type poller struct {
	client *spotify.Client
	hub    *hub

	// mu protects all fields below; both the poll loop and the lyrics-fetch
	// goroutine access them.
	mu sync.Mutex

	// Playback anchor: the position and the time at which it was recorded.
	// Used to extrapolate current position between polls.
	lastTrackID   string
	lastIsPlaying bool
	lastPos       int64
	lastPollAt    time.Time

	// Current broadcast state.
	currentTrack  *spotify.Track
	currentLyrics []lyricLine

	// Set to true once a lyrics fetch has been attempted for lastTrackID,
	// preventing duplicate fetches.
	lyricsFetched bool

	retryDelay time.Duration
}

func newPoller(client *spotify.Client, h *hub) *poller {
	return &poller{client: client, hub: h}
}

// estimatePos returns the estimated current playback position in milliseconds.
// Must be called with p.mu held.
func (p *poller) estimatePos() int64 {
	if p.lastIsPlaying && !p.lastPollAt.IsZero() {
		return p.lastPos + time.Since(p.lastPollAt).Milliseconds()
	}
	return p.lastPos
}

// broadcastPlaying assembles and broadcasts the current playing state.
func (p *poller) broadcastPlaying() {
	p.mu.Lock()
	m := msg{
		Type:       "playing",
		Track:      p.currentTrack,
		Lyrics:     p.currentLyrics,
		PositionMs: p.estimatePos(),
		IsPlaying:  p.lastIsPlaying,
	}
	p.mu.Unlock()
	data, _ := json.Marshal(m)
	p.hub.broadcast(data)
}

// snapshot returns a fresh JSON message reflecting the current playback state,
// with position_ms estimated up to the current moment. Called for each new
// WebSocket client so they join in sync rather than at the last-broadcast position.
func (p *poller) snapshot() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastTrackID == "" {
		return idleJSON
	}
	data, _ := json.Marshal(msg{
		Type:       "playing",
		Track:      p.currentTrack,
		Lyrics:     p.currentLyrics,
		PositionMs: p.estimatePos(),
		IsPlaying:  p.lastIsPlaying,
	})
	return data
}

func (p *poller) run(ctx context.Context) {
	// Fire the first poll immediately, then use the returned interval.
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			timer.Reset(p.poll(ctx))
		}
	}
}

func (p *poller) poll(ctx context.Context) time.Duration {
	state, err := p.client.CurrentlyPlaying(ctx)
	if err != nil {
		var spotifyErr *spotify.Error
		if errors.As(err, &spotifyErr) && spotifyErr.StatusCode == 429 && !spotifyErr.RetryAfter.IsZero() {
			wait := time.Until(spotifyErr.RetryAfter) + time.Second
			log.Printf("rate limited by Spotify, resuming in %v", wait)
			return wait
		}
		p.mu.Lock()
		p.retryDelay = min(p.retryDelay*2+time.Second, 30*time.Second)
		delay := p.retryDelay
		p.mu.Unlock()
		log.Printf("poll error: %v (retry in %v)", err, delay)
		return delay
	}

	p.mu.Lock()
	p.retryDelay = 0
	p.mu.Unlock()

	// Nothing is playing.
	if state == nil || state.Item == nil {
		p.mu.Lock()
		wasActive := p.lastTrackID != ""
		if wasActive {
			p.lastTrackID = ""
			p.lastIsPlaying = false
			p.lastPos = 0
			p.lastPollAt = time.Time{}
			p.currentTrack = nil
			p.currentLyrics = nil
			p.lyricsFetched = false
		}
		p.mu.Unlock()
		if wasActive {
			p.hub.broadcast(idleJSON)
		}
		return pollIntervalIdle
	}

	trackID := state.Item.ID

	p.mu.Lock()
	trackChanged := trackID != p.lastTrackID

	// Seek detection: if the actual position deviates from the extrapolated
	// position by more than the threshold, the user seeked.
	var seeked bool
	if !trackChanged && !p.lastPollAt.IsZero() {
		expected := p.estimatePos()
		diff := state.ProgressMs - expected
		if diff < 0 {
			diff = -diff
		}
		seeked = diff > seekThresholdMs
	}

	playingChanged := state.IsPlaying != p.lastIsPlaying

	// Update the playback anchor and current track.
	p.lastTrackID = trackID
	p.lastIsPlaying = state.IsPlaying
	p.lastPos = state.ProgressMs
	p.lastPollAt = time.Now()
	p.currentTrack = state.Item
	if trackChanged {
		p.currentLyrics = nil
		p.lyricsFetched = false
	}
	p.mu.Unlock()

	if trackChanged {
		// Broadcast immediately with empty lyrics so the widget clears the old
		// track's lines right away; lyrics are fetched in the background.
		p.broadcastPlaying()
		go p.fetchAndBroadcastLyrics(ctx, trackID)
	} else if playingChanged || seeked {
		p.broadcastPlaying()
	}

	if state.IsPlaying {
		return pollIntervalPlaying
	}
	return pollIntervalIdle
}

func (p *poller) fetchAndBroadcastLyrics(ctx context.Context, trackID string) {
	// Claim the fetch slot; bail out if a fetch was already attempted or the
	// track has already changed (e.g. very short track).
	p.mu.Lock()
	if p.lastTrackID != trackID || p.lyricsFetched {
		p.mu.Unlock()
		return
	}
	p.lyricsFetched = true
	var trackDurationMs int64
	if p.currentTrack != nil {
		trackDurationMs = p.currentTrack.DurationMs
	}
	p.mu.Unlock()

	lyricsResp, err := p.client.Lyrics(ctx, trackID)
	if err != nil {
		return // lyrics not available for this track, widget stays blank
	}

	// Parse all lines so we can use their start times as end-time
	// boundaries for the preceding lyric line.
	type rawLine struct {
		startMs int64
		endMs   int64
		words   string
	}
	raw := make([]rawLine, len(lyricsResp.Lyrics.Lines))
	for i, l := range lyricsResp.Lyrics.Lines {
		raw[i].startMs, _ = strconv.ParseInt(l.StartTimeMs, 10, 64)
		raw[i].endMs, _ = strconv.ParseInt(l.EndTimeMs, 10, 64)
		raw[i].words = l.Words
	}

	// Fill in missing end times: use the next line's start time (this matches
	// how Spotify's own player determines when each line ends).
	for i := 0; i < len(raw)-1; i++ {
		if raw[i].endMs == 0 {
			raw[i].endMs = raw[i+1].startMs
		}
	}
	if len(raw) > 0 && raw[len(raw)-1].endMs == 0 {
		raw[len(raw)-1].endMs = trackDurationMs
	}

	lines := make([]lyricLine, len(raw))
	for i, l := range raw {
		lines[i] = lyricLine{StartMs: l.startMs, EndMs: l.endMs, Words: l.words}
	}

	p.mu.Lock()
	if p.lastTrackID != trackID {
		p.mu.Unlock()
		return // track changed while we were fetching
	}
	p.currentLyrics = lines
	p.mu.Unlock()

	p.broadcastPlaying()
}
