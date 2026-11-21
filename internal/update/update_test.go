package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func marked(tag, obsMin string) Release {
	return Release{
		TagName: tag,
		HTMLURL: "https://github.com/o/r/releases/tag/" + tag,
		Body:    fmt.Sprintf("## Changes\n\n<!-- obs-compat {\"obs_sdk\":\"%s\",\"obs_min\":\"%s\"} -->\n", obsMin, obsMin),
	}
}

func unmarked(tag string) Release {
	return Release{TagName: tag, Body: "## Changes\n\nNothing machine readable here."}
}

func TestChooseOffersCompatibleNewer(t *testing.T) {
	d := Choose([]Release{marked("v0.3.4", "32.0.0")}, "v0.3.3", "32.2.2")
	if d.Update == nil || d.Update.TagName != "v0.3.4" {
		t.Fatalf("expected v0.3.4, got %+v", d.Update)
	}
	if d.TooNew != nil {
		t.Fatalf("unexpected TooNew %v", d.TooNew.TagName)
	}
}

// The case the whole mechanism exists for: OBS 32 host, newest release needs 33.
func TestChooseBlocksReleaseNeedingNewerOBS(t *testing.T) {
	d := Choose([]Release{marked("v0.4.0", "33.0.0")}, "v0.3.3", "32.0.2")
	if d.Update != nil {
		t.Fatalf("must not offer an OBS 33 build to OBS 32, got %v", d.Update.TagName)
	}
	if d.TooNew == nil || d.TooNew.TagName != "v0.4.0" || d.NeedsOBS != "33.0.0" {
		t.Fatalf("expected TooNew v0.4.0 needing 33.0.0, got %+v", d)
	}
}

func TestChoosePrefersCompatibleMaintenanceRelease(t *testing.T) {
	releases := []Release{
		marked("v0.4.0", "33.0.0"), // created first, so it leads the list
		marked("v0.3.4", "32.0.0"),
	}
	d := Choose(releases, "v0.3.3", "32.0.2")
	if d.Update == nil || d.Update.TagName != "v0.3.4" {
		t.Fatalf("expected v0.3.4 as the offer, got %+v", d.Update)
	}
	if d.TooNew == nil || d.TooNew.TagName != "v0.4.0" {
		t.Fatalf("expected v0.4.0 reported as too new, got %+v", d.TooNew)
	}
}

// Releases published before the marker exists carry no constraint; dropping them
// would strand a user on v0.3.2 from ever being offered v0.3.3.
func TestChooseStillOffersUnmarkedReleases(t *testing.T) {
	d := Choose([]Release{unmarked("v0.3.3")}, "v0.3.2", "32.0.2")
	if d.Update == nil || d.Update.TagName != "v0.3.3" {
		t.Fatalf("expected unmarked v0.3.3 to stay offerable, got %+v", d.Update)
	}
}

func TestChooseSkipsMalformedMarker(t *testing.T) {
	r := Release{TagName: "v0.9.0", Body: "<!-- obs-compat {obs_min: } -->"}
	d := Choose([]Release{r}, "v0.3.3", "32.0.2")
	if d.Update != nil {
		t.Fatalf("malformed marker must not be offered, got %v", d.Update.TagName)
	}
	if len(d.Malformed) != 1 || d.Malformed[0] != "v0.9.0" {
		t.Fatalf("expected v0.9.0 reported malformed, got %v", d.Malformed)
	}
}

// GitHub's releases/latest hides drafts and pre-releases; the list endpoint does
// not, so the filter has to be explicit.
func TestChooseIgnoresDraftsAndPrereleases(t *testing.T) {
	pre := marked("v0.4.0", "32.0.0")
	pre.Prerelease = true
	draft := marked("v0.5.0", "32.0.0")
	draft.Draft = true
	d := Choose([]Release{pre, draft}, "v0.3.3", "32.0.2")
	if d.Update != nil {
		t.Fatalf("drafts and pre-releases must not be offered, got %v", d.Update.TagName)
	}
}

func TestChoosePicksHighestSemverNotFirstEntry(t *testing.T) {
	// Creation order in the list is newest-created first, not highest version.
	releases := []Release{marked("v0.3.4", "32.0.0"), marked("v0.3.10", "32.0.0")}
	d := Choose(releases, "v0.3.3", "32.0.2")
	if d.Update == nil || d.Update.TagName != "v0.3.10" {
		t.Fatalf("expected v0.3.10, got %+v", d.Update)
	}
}

func TestChooseNeverOffersDowngrade(t *testing.T) {
	d := Choose([]Release{unmarked("v0.3.3")}, "v0.3.4", "32.0.2")
	if d.Update != nil || d.TooNew != nil {
		t.Fatalf("a yanked or older release must not be offered, got %+v", d)
	}
}

// A build with no tag reports dev, which is not semver and cannot be compared.
// A build like v0.3.3-9-g78aa8e8 is a valid semver prerelease and is still
// compared, which is how the plugin has behaved so far.
func TestChooseIgnoresUnparsableCurrent(t *testing.T) {
	d := Choose([]Release{marked("v0.4.0", "32.0.0")}, "dev", "32.0.2")
	if d.Update != nil || d.TooNew != nil {
		t.Fatalf("expected no offer for an unparsable current version, got %+v", d)
	}

	d = Choose([]Release{marked("v0.4.0", "32.0.0")}, "v0.3.3-9-g78aa8e8", "32.0.2")
	if d.Update == nil || d.Update.TagName != "v0.4.0" {
		t.Fatalf("expected a dev build to still be offered a compatible update, got %+v", d.Update)
	}
}

func TestCompatOf(t *testing.T) {
	c, ok, err := marked("v1.0.0", "32.0.0").CompatOf()
	if err != nil || !ok || c.ObsMin != "32.0.0" {
		t.Fatalf("marked release: ok=%v compat=%+v err=%v", ok, c, err)
	}
	if _, ok, err := unmarked("v1.0.0").CompatOf(); ok || err != nil {
		t.Fatalf("unmarked release must report no constraint: ok=%v err=%v", ok, err)
	}
	if _, ok, err := (Release{Body: "<!-- obs-compat oops -->"}).CompatOf(); ok || err == nil {
		t.Fatal("marker present but unparsable must report an error")
	}
}

// The release job writes this exact text; if the format changes there, the
// parser has to follow, so the contract is pinned here.
func TestCompatOfReadsGeneratedReleaseBody(t *testing.T) {
	body := "Built against the OBS 33.0.2 SDK. Requires OBS 33.0.0 or newer.\n\n" +
		"<!-- obs-compat {\"obs_sdk\":\"33.0.2\",\"obs_min\":\"33.0.0\"} -->\n"
	c, ok, err := (Release{TagName: "v0.4.0", Body: body}).CompatOf()
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if c.ObsSDK != "33.0.2" || c.ObsMin != "33.0.0" {
		t.Fatalf("got %+v", c)
	}

	d := Choose([]Release{{TagName: "v0.4.0", Body: body}}, "v0.3.4", "32.0.2")
	if d.Update != nil || d.TooNew == nil || d.NeedsOBS != "33.0.0" {
		t.Fatalf("OBS 32 host must not be offered this release: %+v", d)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"32.0.0", "32.0.2", -1},
		{"32.0.2", "32.0.0", 1},
		{"32.2.2", "32.2.2", 0},
		{"32", "32.0.2", -1},
		{"33", "32.9.9", 1},
		{"garbage", "32.0.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestOBSVersionDecodesLibobsPacking(t *testing.T) {
	// libobs: (major << 24) | (minor << 16) | patch
	raw := uint32(33)<<24 | uint32(0)<<16 | 2
	if got := OBSVersion(raw); got != "33.0.2" {
		t.Fatalf("got %q, want 33.0.2", got)
	}
}

func TestNextPageURL(t *testing.T) {
	link := `<https://api.github.com/repos/o/r/releases?per_page=2&page=2>; rel="next", <https://api.github.com/repos/o/r/releases?per_page=2&page=3>; rel="last"`
	if got := nextPageURL(link); got != "https://api.github.com/repos/o/r/releases?per_page=2&page=2" {
		t.Fatalf("got %q", got)
	}
	if got := nextPageURL(`<https://api.github.com/x>; rel="prev"`); got != "" {
		t.Fatalf("expected no next link, got %q", got)
	}
}

func TestListFollowsPagination(t *testing.T) {
	pages := map[string][]Release{
		"/first":  {unmarked("v0.2.0"), unmarked("v0.3.0")},
		"/second": {unmarked("v0.3.3")},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first":
			w.Header().Set("Link", fmt.Sprintf("<%s/second>; rel=\"next\"", "http://"+r.Host))
		case "/second":
			// no Link header: last page
		default:
			http.NotFound(w, r)
			return
		}
		if err := writeJSON(w, pages[r.URL.Path]); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	got, err := List(srv.URL+"/first", "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 releases across pages, got %d: %+v", len(got), got)
	}
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

func TestListPropagatesHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := List(srv.URL, "test"); err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
}
