package spotify

import (
	"math"
	"strconv"
	"testing"
	"time"
)

// stateTs is an arbitrary point in time; player timestamps live in Spotify's
// clock domain, so the local clock only matters in the fallback cases.
const stateTs = int64(1_700_000_000_000)

func speedPtr(s float64) *float64 { return &s }

// playerState builds a playing device state reporting posMs as of stateTs.
func playerState(posMs int64, speed *float64, durMs int64) connectStatePlayerState {
	state := connectStatePlayerState{
		IsPlaying:             true,
		PositionAsOfTimestamp: strconv.FormatInt(posMs, 10),
		Timestamp:             strconv.FormatInt(stateTs, 10),
		PlaybackSpeed:         speed,
	}
	if durMs > 0 {
		state.Track.Metadata = connectStateMeta{"duration": strconv.FormatInt(durMs, 10)}
	}
	return state
}

func TestComputeProgress(t *testing.T) {
	tests := []struct {
		name            string
		state           connectStatePlayerState
		mutate          func(*connectStatePlayerState)
		serverTimestamp int64
		want            int64
	}{
		{
			name:            "normal speed",
			state:           playerState(1000, speedPtr(1.0), 0),
			serverTimestamp: stateTs + 2000,
			want:            3000,
		},
		{
			name:            "missing speed defaults to normal",
			state:           playerState(1000, nil, 0),
			serverTimestamp: stateTs + 2000,
			want:            3000,
		},
		{
			name:            "negative speed defaults to normal",
			state:           playerState(1000, speedPtr(-1), 0),
			serverTimestamp: stateTs + 2000,
			want:            3000,
		},
		{
			name:            "nan speed defaults to normal",
			state:           playerState(1000, speedPtr(math.NaN()), 0),
			serverTimestamp: stateTs + 2000,
			want:            3000,
		},
		{
			// A buffering or starting device reports is_playing with speed 0.
			// Extrapolating over that startup window is what produced the drift.
			name:            "stalled device does not advance",
			state:           playerState(0, speedPtr(0.0), 0),
			serverTimestamp: stateTs + 16_000,
			want:            0,
		},
		{
			name:            "half speed",
			state:           playerState(1000, speedPtr(0.5), 0),
			serverTimestamp: stateTs + 2000,
			want:            2000,
		},
		{
			// Without a server timestamp the local clock is the only reference.
			name:            "absent server timestamp falls back to local clock",
			state:           playerState(1000, speedPtr(1.0), 0),
			serverTimestamp: 0,
			want:            6000,
		},
		{
			name:            "non-positive server timestamp falls back to local clock",
			state:           playerState(1000, speedPtr(1.0), 0),
			serverTimestamp: -5,
			want:            6000,
		},
		{
			// The player timestamp can be ahead of the server timestamp by a few
			// hundred ms; that must not pull the position backwards.
			name:            "negative skew clamps elapsed to zero",
			state:           playerState(1000, speedPtr(1.0), 0),
			serverTimestamp: stateTs - 700,
			want:            1000,
		},
		{
			name:            "paused device keeps reported position",
			state:           playerState(1000, speedPtr(1.0), 0),
			mutate:          func(s *connectStatePlayerState) { s.IsPaused = true },
			serverTimestamp: stateTs + 20_000,
			want:            1000,
		},
		{
			name:            "stopped device keeps reported position",
			state:           playerState(1000, speedPtr(1.0), 0),
			mutate:          func(s *connectStatePlayerState) { s.IsPlaying = false },
			serverTimestamp: stateTs + 20_000,
			want:            1000,
		},
		{
			name:            "clamped to duration",
			state:           playerState(175_000, speedPtr(1.0), 180_000),
			serverTimestamp: stateTs + 10_000,
			want:            180_000,
		},
		{
			name:            "negative position clamped to zero",
			state:           playerState(-500, speedPtr(1.0), 180_000),
			serverTimestamp: stateTs,
			want:            0,
		},
		{
			// A negative position is never valid, with or without a duration.
			name:            "negative position clamped to zero without duration",
			state:           playerState(-500, speedPtr(1.0), 0),
			serverTimestamp: stateTs,
			want:            0,
		},
		{
			name:            "stalled device clamps negative position without duration",
			state:           playerState(-500, speedPtr(0.0), 0),
			serverTimestamp: stateTs + 16_000,
			want:            0,
		},
		{
			name:            "unknown duration does not clamp the upper bound",
			state:           playerState(175_000, speedPtr(1.0), 0),
			serverTimestamp: stateTs + 10_000,
			want:            185_000,
		},
		{
			name:            "unparseable position",
			state:           playerState(0, speedPtr(1.0), 0),
			mutate:          func(s *connectStatePlayerState) { s.PositionAsOfTimestamp = "" },
			serverTimestamp: stateTs + 2000,
			want:            0,
		},
		{
			name:            "unparseable player timestamp falls back to position",
			state:           playerState(1000, speedPtr(1.0), 0),
			mutate:          func(s *connectStatePlayerState) { s.Timestamp = "" },
			serverTimestamp: stateTs + 2000,
			want:            1000,
		},
	}

	now := time.UnixMilli(stateTs + 5000)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state
			if tt.mutate != nil {
				tt.mutate(&state)
			}
			if got := computeProgress(state, tt.serverTimestamp, now); got != tt.want {
				t.Errorf("computeProgress() = %d ms, want %d ms", got, tt.want)
			}
		})
	}
}

func TestIsAdvancing(t *testing.T) {
	tests := []struct {
		name   string
		state  connectStatePlayerState
		mutate func(*connectStatePlayerState)
		want   bool
	}{
		{name: "playing", state: playerState(0, speedPtr(1.0), 0), want: true},
		{name: "playing without speed", state: playerState(0, nil, 0), want: true},
		{name: "stalled", state: playerState(0, speedPtr(0.0), 0), want: false},
		{
			name:   "paused",
			state:  playerState(0, speedPtr(1.0), 0),
			mutate: func(s *connectStatePlayerState) { s.IsPaused = true },
			want:   false,
		},
		{
			name:   "not playing",
			state:  playerState(0, speedPtr(1.0), 0),
			mutate: func(s *connectStatePlayerState) { s.IsPlaying = false },
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.state
			if tt.mutate != nil {
				tt.mutate(&state)
			}
			if got := isAdvancing(state); got != tt.want {
				t.Errorf("isAdvancing() = %v, want %v", got, tt.want)
			}
		})
	}
}
