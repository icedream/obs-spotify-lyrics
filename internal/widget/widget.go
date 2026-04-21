// Package widget embeds the default OBS browser-source widget HTML.
// The file is intended as a starting point for customisation; users are
// encouraged to copy and adapt it to match their own overlay theme.
package widget

import _ "embed"

// HTML is the default lyrics widget page served by the built-in server.
//
//go:embed widget.html
var HTML []byte
