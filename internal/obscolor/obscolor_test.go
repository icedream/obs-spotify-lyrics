package obscolor_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/icedream/spotify-lyrics-widget/internal/obscolor"
)

// OBS ABGR format: 0xAABBGGRR

func TestFromCSS(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   uint32
		wantOK bool
	}{
		// #rgb shorthand — alpha always 0xFF
		{name: "#rgb white", input: "#fff", want: 0xFFFFFFFF, wantOK: true},
		{name: "#rgb black", input: "#000", want: 0xFF000000, wantOK: true},
		{name: "#rgb red", input: "#f00", want: 0xFF0000FF, wantOK: true},
		{name: "#rgb green", input: "#0f0", want: 0xFF00FF00, wantOK: true},
		{name: "#rgb blue", input: "#00f", want: 0xFFFF0000, wantOK: true},

		// #rrggbb — alpha always 0xFF
		{name: "#rrggbb white", input: "#ffffff", want: 0xFFFFFFFF, wantOK: true},
		{name: "#rrggbb black", input: "#000000", want: 0xFF000000, wantOK: true},
		{name: "#rrggbb red", input: "#ff0000", want: 0xFF0000FF, wantOK: true},
		{name: "#rrggbb green", input: "#00ff00", want: 0xFF00FF00, wantOK: true},
		{name: "#rrggbb blue", input: "#0000ff", want: 0xFFFF0000, wantOK: true},

		// #rrggbbaa — CSS aa is alpha
		{name: "#rrggbbaa fully opaque white", input: "#ffffffff", want: 0xFFFFFFFF, wantOK: true},
		{name: "#rrggbbaa fully transparent white", input: "#ffffff00", want: 0x00FFFFFF, wantOK: true},
		{name: "#rrggbbaa 50% transparent red", input: "#ff000080", want: 0x800000FF, wantOK: true},

		// rgb()
		{name: "rgb() white", input: "rgb(255, 255, 255)", want: 0xFFFFFFFF, wantOK: true},
		{name: "rgb() black", input: "rgb(0, 0, 0)", want: 0xFF000000, wantOK: true},
		{name: "rgb() red", input: "rgb(255, 0, 0)", want: 0xFF0000FF, wantOK: true},
		{name: "rgb() no spaces", input: "rgb(255,0,0)", want: 0xFF0000FF, wantOK: true},

		// rgba() with fractional alpha (0.0-1.0)
		{name: "rgba() fully opaque white", input: "rgba(255, 255, 255, 1)", want: 0xFFFFFFFF, wantOK: true},
		{name: "rgba() fully transparent white", input: "rgba(255, 255, 255, 0)", want: 0x00FFFFFF, wantOK: true},
		{name: "rgba() half-alpha white", input: "rgba(255, 255, 255, 0.5)", want: 0x80FFFFFF, wantOK: true},
		// actual default from widget.html for adjacent lines
		{name: "rgba() adjacent line default", input: "rgba(255, 255, 255, 0.45)", want: 0x73FFFFFF, wantOK: true},

		// whitespace trimming
		{name: "leading/trailing spaces", input: "  #ffffff  ", want: 0xFFFFFFFF, wantOK: true},

		// invalid inputs
		{name: "empty string", input: "", wantOK: false},
		{name: "garbage", input: "notacolor", wantOK: false},
		{name: "#gg invalid hex", input: "#gggggg", wantOK: false},
		{name: "#5 wrong length", input: "#12345", wantOK: false},
		{name: "rgba() missing components", input: "rgba(255, 255)", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := obscolor.FromCSS(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equalf(t, tt.want, got, "FromCSS(%q) = 0x%08X, want 0x%08X", tt.input, got, tt.want)
			}
		})
	}
}

func TestToCSS(t *testing.T) {
	tests := []struct {
		name  string
		input uint32
		want  string
	}{
		{name: "fully opaque white", input: 0xFFFFFFFF, want: "rgba(255, 255, 255, 1)"},
		{name: "fully opaque black", input: 0xFF000000, want: "rgba(0, 0, 0, 1)"},
		{name: "fully transparent black", input: 0x00000000, want: "rgba(0, 0, 0, 0)"},
		{name: "fully transparent white", input: 0x00FFFFFF, want: "rgba(255, 255, 255, 0)"},
		{name: "fully opaque red", input: 0xFF0000FF, want: "rgba(255, 0, 0, 1)"},
		{name: "fully opaque green", input: 0xFF00FF00, want: "rgba(0, 255, 0, 1)"},
		{name: "fully opaque blue", input: 0xFFFF0000, want: "rgba(0, 0, 255, 1)"},
		{name: "half alpha", input: 0x80FFFFFF, want: "rgba(255, 255, 255, 0.501961)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := obscolor.ToCSS(tt.input)
			assert.Equalf(t, tt.want, got, "ToCSS(0x%08X)", tt.input)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	inputs := []string{
		"#ffffff",
		"#000000",
		"#ff0000",
		"#ffffffff",
		"#ffffff00",
		"rgba(255, 255, 255, 0.45)",
		"rgba(255, 255, 255, 1)",
		"rgba(0, 0, 0, 0)",
	}
	for _, css := range inputs {
		t.Run(css, func(t *testing.T) {
			obsVal, ok := obscolor.FromCSS(css)
			require.Truef(t, ok, "FromCSS(%q) returned false", css)

			back := obscolor.ToCSS(obsVal)
			obsVal2, ok2 := obscolor.FromCSS(back)
			require.Truef(t, ok2, "FromCSS(ToCSS(%q)) = %q, which failed to parse", css, back)

			assert.Equalf(t, obsVal, obsVal2, "round-trip mismatch for %q: 0x%08X -> %q -> 0x%08X", css, obsVal, back, obsVal2)
		})
	}
}
