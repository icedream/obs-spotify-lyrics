// Package obscolor converts between CSS color strings and OBS's native ABGR
// uint32 color format (0xAABBGGRR: low byte = red, high byte = alpha).
package obscolor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// FromCSS parses a CSS color string and returns the OBS ABGR uint32 value.
// Supports #rgb, #rrggbb, #rrggbbaa, rgb(), and rgba() formats.
// Returns (0, false) if the input cannot be parsed.
func FromCSS(css string) (uint32, bool) {
	css = strings.TrimSpace(css)
	if strings.HasPrefix(css, "#") {
		hex := strings.TrimPrefix(css, "#")
		switch len(hex) {
		case 3:
			r, err1 := strconv.ParseUint(string([]byte{hex[0], hex[0]}), 16, 8)
			g, err2 := strconv.ParseUint(string([]byte{hex[1], hex[1]}), 16, 8)
			b, err3 := strconv.ParseUint(string([]byte{hex[2], hex[2]}), 16, 8)
			if err1 != nil || err2 != nil || err3 != nil {
				return 0, false
			}
			return 0xFF000000 | uint32(b)<<16 | uint32(g)<<8 | uint32(r), true
		case 6:
			v, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return 0, false
			}
			r, g, b := (v>>16)&0xFF, (v>>8)&0xFF, v&0xFF
			return 0xFF000000 | uint32(b)<<16 | uint32(g)<<8 | uint32(r), true
		case 8:
			// CSS #rrggbbaa: rr=high byte, aa=low byte
			v, err := strconv.ParseUint(hex, 16, 32)
			if err != nil {
				return 0, false
			}
			r, g, b, a := (v>>24)&0xFF, (v>>16)&0xFF, (v>>8)&0xFF, v&0xFF
			return uint32(a)<<24 | uint32(b)<<16 | uint32(g)<<8 | uint32(r), true
		}
		return 0, false
	}

	// rgb() / rgba()
	css = strings.TrimSuffix(css, ")")
	var inner string
	if after, ok := strings.CutPrefix(css, "rgba("); ok {
		inner = after
	} else if after, ok := strings.CutPrefix(css, "rgb("); ok {
		inner = after
	} else {
		return 0, false
	}
	parts := strings.SplitN(inner, ",", 4)
	if len(parts) < 3 {
		return 0, false
	}
	r, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	g, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	b, err3 := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	a := 255.0
	if len(parts) == 4 {
		av, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		if err != nil {
			return 0, false
		}
		if av <= 1.0 {
			av *= 255.0
		}
		a = av
	}
	return uint32(math.Round(a))<<24 | uint32(math.Round(b))<<16 | uint32(math.Round(g))<<8 | uint32(math.Round(r)), true
}

// ToCSS converts an OBS ABGR uint32 color value to a CSS rgba() string.
// OBS stores colors as 0xAABBGGRR (low byte = red, high byte = alpha).
func ToCSS(val uint32) string {
	r := val & 0xFF
	g := (val >> 8) & 0xFF
	b := (val >> 16) & 0xFF
	a := (val >> 24) & 0xFF
	return fmt.Sprintf("rgba(%d, %d, %d, %.6g)", r, g, b, float64(a)/255.0)
}
