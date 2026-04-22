// Package widget embeds the default OBS browser-source widget HTML.
// The file is intended as a starting point for customisation; users are
// encouraged to copy and adapt it to match their own overlay theme.
package widget

import (
	"strings"
	_ "embed"
)

// HTML is the default lyrics widget page served by the built-in server.
//
//go:embed widget.html
var HTML []byte

// CSSVarDef describes a single CSS custom property defined in the widget's
// :root block and annotated with /* obs: Label */.
type CSSVarDef struct {
	Key    string // OBS settings key, e.g. "css_font_family"
	Prop   string // CSS custom property, e.g. "--font-family"
	Label  string // human-readable OBS UI label, e.g. "Font family"
	DefVal string // default value as written in :root, e.g. "'Segoe UI', system-ui, sans-serif"
}

// CSSVars is the ordered list of CSS variables parsed from the widget HTML at
// package init time. Only lines annotated with /* obs: Label */ are included.
var CSSVars []CSSVarDef

func init() { CSSVars = ParseCSSVars(HTML) }

// ParseCSSVars scans html for lines of the form
//
//	--property: value; /* obs: Label */
//
// and returns a CSSVarDef for each. Lines without the annotation are skipped.
// The OBS settings key is derived mechanically: "--font-family" -> "css_font_family".
func ParseCSSVars(html []byte) []CSSVarDef {
	var out []CSSVarDef
	for _, line := range strings.Split(string(html), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "--") {
			continue
		}
		obsIdx := strings.Index(trimmed, "/* obs:")
		if obsIdx < 0 {
			continue
		}
		label := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed[obsIdx:], "/* obs:"), "*/"))

		// Extract property name and value from the part before the comment.
		decl := strings.TrimSpace(trimmed[:obsIdx])
		decl = strings.TrimSuffix(decl, ";")
		colonIdx := strings.Index(decl, ":")
		if colonIdx < 0 {
			continue
		}
		prop := strings.TrimSpace(decl[:colonIdx])
		val := strings.TrimSpace(decl[colonIdx+1:])

		key := "css_" + strings.ReplaceAll(strings.TrimPrefix(prop, "--"), "-", "_")
		out = append(out, CSSVarDef{Key: key, Prop: prop, Label: label, DefVal: val})
	}
	return out
}
