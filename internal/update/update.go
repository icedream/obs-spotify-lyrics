// Package update decides which GitHub release to offer a plugin install, given
// the OBS version that install is running inside.
//
// A release built against an OBS N SDK is refused by OBS versions older than N
// (libobs/obs-module.c rejects any plugin whose obs_module_ver() major.minor
// exceeds the host's LIBOBS_API_VER from OBS 32.0.2 on), so offering the newest
// release unconditionally can install a plugin the host will not load.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Asset is a GitHub release asset.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is a GitHub release, trimmed to what an update check reads.
type Release struct {
	TagName    string  `json:"tag_name"`
	HTMLURL    string  `json:"html_url"`
	Body       string  `json:"body"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}

// Compat is the OBS support window a release advertises in its body.
type Compat struct {
	ObsSDK string `json:"obs_sdk"`
	ObsMin string `json:"obs_min"`
}

// Releases advertise their window with an HTML comment so the marker is
// invisible on the release page but arrives with the release JSON.
const markerText = "<!-- obs-compat "

var markerRE = regexp.MustCompile(`(?s)<!--\s*obs-compat\s*(\{.*?\})\s*-->`)

// CompatOf reads the advertised window. ok is false when the release carries no
// marker at all, which is how releases predating the marker are recognised: they
// carry no constraint, so they stay offerable. err is non-nil when a marker is
// present but unreadable, which must never be treated as "unconstrained".
func (r Release) CompatOf() (c Compat, ok bool, err error) {
	m := markerRE.FindStringSubmatch(r.Body)
	if m == nil {
		if strings.Contains(r.Body, markerText) {
			return Compat{}, false, fmt.Errorf("unparsable obs-compat marker")
		}
		return Compat{}, false, nil
	}
	if err := json.Unmarshal([]byte(m[1]), &c); err != nil {
		return Compat{}, true, fmt.Errorf("obs-compat marker: %w", err)
	}
	return c, true, nil
}

// Version is the tag in the form golang.org/x/mod/semver expects.
func (r Release) Version() string { return normalizeVersion(r.TagName) }

func normalizeVersion(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

// Decision is the outcome of an update check.
type Decision struct {
	// Update is the newest release that is newer than the running version and
	// loadable on the host OBS.
	Update *Release
	// TooNew is the newest release that is newer than the running version but
	// needs a newer OBS than the host runs. It is reported so the user can be
	// told to update OBS first instead of being handed an installer whose
	// plugin the host will then refuse.
	TooNew *Release
	// NeedsOBS is the OBS floor advertised by TooNew.
	NeedsOBS string
	// Malformed lists tags whose marker could not be read. Such releases are
	// never offered.
	Malformed []string
}

// Choose selects from releases. current is the running plugin version and
// hostOBS the running OBS version, for example "32.0.2".
//
// GitHub orders releases by creation date, not by version, so the newest
// release is found by comparing semver and never by position in the list.
func Choose(releases []Release, current, hostOBS string) Decision {
	var d Decision

	cur := normalizeVersion(current)
	if !semver.IsValid(cur) {
		// A checkout with no tag at all reports dev, which cannot be compared.
		return d
	}

	for i := range releases {
		r := releases[i]
		if r.Draft || r.Prerelease {
			continue
		}
		ver := r.Version()
		if !semver.IsValid(ver) || semver.Compare(ver, cur) <= 0 {
			// Also what keeps a yanked release from prompting a downgrade.
			continue
		}

		c, marked, err := r.CompatOf()
		if err != nil {
			d.Malformed = append(d.Malformed, r.TagName)
			continue
		}
		if marked && c.ObsMin != "" && CompareVersions(c.ObsMin, hostOBS) > 0 {
			if d.TooNew == nil || semver.Compare(ver, d.TooNew.Version()) > 0 {
				d.TooNew, d.NeedsOBS = &releases[i], c.ObsMin
			}
			continue
		}
		if d.Update == nil || semver.Compare(ver, d.Update.Version()) > 0 {
			d.Update = &releases[i]
		}
	}

	return d
}

// CompareVersions compares OBS versions that need not be three parts wide:
// "32", "32.0" and "32.0.2" all compare correctly. Returns -1, 0 or 1.
func CompareVersions(a, b string) int {
	x, xok := versionTriple(a)
	y, yok := versionTriple(b)
	if !xok || !yok {
		return 0
	}
	for i := range 3 {
		if x[i] != y[i] {
			if x[i] < y[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionTriple(s string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "v")), ".")
	if len(parts) == 0 || parts[0] == "" {
		return out, false
	}
	for i, p := range parts {
		if i > 2 {
			break
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// OBSVersion turns the uint32 from obs_get_version() into a version string.
// libobs packs it as (major << 24) | (minor << 16) | patch.
func OBSVersion(raw uint32) string {
	return fmt.Sprintf("%d.%d.%d", raw>>24, (raw>>16)&0xFF, raw&0xFF)
}

// List fetches releases, following GitHub's Link pagination so a project with
// more releases than fit in one page is still seen in full.
func List(url, userAgent string) ([]Release, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	var all []Release
	for page := 0; url != "" && page < 10; page++ {
		batch, next, err := listPage(client, url, userAgent)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		url = next
	}
	return all, nil
}

// listPage fetches one releases page and returns its contents plus the URL of
// the next page, empty once GitHub stops advertising one.
func listPage(client *http.Client, url, userAgent string) ([]Release, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var batch []Release
	if err := json.NewDecoder(resp.Body).Decode(&batch); err != nil {
		return nil, "", err
	}
	return batch, nextPageURL(resp.Header.Get("Link")), nil
}

// nextPageURL extracts rel="next" from a GitHub Link header.
func nextPageURL(link string) string {
	for _, part := range strings.Split(link, ",") {
		fields := strings.Split(part, ";")
		if len(fields) < 2 || !strings.Contains(fields[1], `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(fields[0]), "<>")
	}
	return ""
}
