package spotify

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// tokenURL is where we authenticate against.
	tokenURL = "https://open.spotify.com/api/token"

	// serverTimeURL is where we fetch the server time on Spotify's end from.
	serverTimeURL = "https://open.spotify.com/api/server-time"

	// secretKeyURL is the URL to fetch the secret needed for server-time based
	// login.
	secretKeyURL = "https://raw.githubusercontent.com/xyloflake/spot-secrets-go/main/secrets/secretDict.json"

	// tokenExpiryBuffer is subtracted from the token expiry time to refresh
	// slightly early and avoid using a token that is about to expire.
	tokenExpiryBuffer = 5 * time.Second
)

// Client handles authentication and communication with Spotify's internal APIs.
type Client struct {
	// spDC contains the sp_dc cookie value to transmit for token requests.
	spDC string

	// deviceID is a randomly generated device ID.
	deviceID string

	// httpClient is the general HTTP client used for API requests
	httpClient *http.Client

	// tokenClient is an HTTP client that does not follow redirects,
	// specifically used for token authentication.
	tokenClient *http.Client

	// token holds the authenticated token.
	token *tokenResponse

	// mu is this client's general lock.
	mu sync.Mutex
}

// NewClient creates a new Spotify client with the given sp_dc cookie value.
//
// deviceID can be left empty to use a randomly generarted device ID.
func NewClient(spDC, deviceID string) *Client {
	noRedirect := func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// generate random device ID if none was explicitly passed
	if len(deviceID) == 0 {
		var devIDBytes [8]byte
		rand.Read(devIDBytes[:])
		deviceID = hex.EncodeToString(devIDBytes[:])
	}

	return &Client{
		spDC:        spDC,
		deviceID:    deviceID,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		tokenClient: &http.Client{Timeout: 30 * time.Second, CheckRedirect: noRedirect},
	}
}

// getLatestSecretKeyVersion fetches the secret key dictionary and returns the
// XOR-transformed secret string and its version.
//
// The secret is formed by XOR-ing each integer in the array with ((i%33)+9),
// then concatenating the decimal string representation of each result.
func (c *Client) getLatestSecretKeyVersion() (string, string, error) {
	resp, err := c.httpClient.Get(secretKeyURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch secret key: %w", err)
	}
	defer resp.Body.Close()

	version, originalSecret, err := decodeLastSecretEntry(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse secret key response: %w", err)
	}

	transformed := make([]string, len(originalSecret))
	for i, char := range originalSecret {
		transformed[i] = strconv.Itoa(char ^ ((i % 33) + 9))
	}

	return strings.Join(transformed, ""), version, nil
}

// decodeLastSecretEntry reads the JSON object from r and returns the last key
// and its integer-array value, preserving the insertion order of keys.
func decodeLastSecretEntry(r io.Reader) (string, []int, error) {
	dec := json.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return "", nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return "", nil, errors.New("expected JSON object")
	}

	var lastKey string
	var lastValue []int

	for dec.More() {
		tok, err = dec.Token()
		if err != nil {
			return "", nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return "", nil, errors.New("expected string key in secrets object")
		}

		var val []int
		if err := dec.Decode(&val); err != nil {
			return "", nil, fmt.Errorf("failed to decode secret for version %q: %w", key, err)
		}
		lastKey = key
		lastValue = val
	}

	if lastKey == "" {
		return "", nil, errors.New("secret key dictionary is empty")
	}

	return lastKey, lastValue, nil
}

// generateTOTP produces a 6-digit TOTP code from the server time and the
// pre-transformed secret string.  The algorithm is intentionally non-standard:
// the secret is used as raw bytes of its decimal string representation, not as
// a base32-decoded value.
//
// ref https://github.com/akashrchandran/spotify-lyrics-api/blob/4b0858496a83b235c7aeeff43b8b77590d9c9b98/src/Spotify.php#L48-L74
func generateTOTP(serverTimeSeconds int64, secret string) string {
	const period = 30
	const digits = 6

	counter := serverTimeSeconds / period
	counterBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(counterBytes, uint64(counter))

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(counterBytes)
	h := mac.Sum(nil)

	offset := h[len(h)-1] & 0x0F
	code := (int64(h[offset]&0x7F) << 24) |
		(int64(h[offset+1]&0xFF) << 16) |
		(int64(h[offset+2]&0xFF) << 8) |
		int64(h[offset+3]&0xFF)

	code %= 1_000_000 // 10^digits
	return fmt.Sprintf("%06d", code)
}

// getServerTimeParams fetches Spotify's current server time and builds the
// query parameters required for the token endpoint.
func (c *Client) getServerTimeParams() (url.Values, error) {
	resp, err := c.httpClient.Get(serverTimeURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server time: %w", err)
	}
	defer resp.Body.Close()

	var st serverTimeResponse
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, fmt.Errorf("failed to decode server time response: %w", err)
	}

	secret, version, err := c.getLatestSecretKeyVersion()
	if err != nil {
		return nil, err
	}

	totp := generateTOTP(st.ServerTime, secret)

	params := url.Values{}
	params.Set("reason", "transport")
	params.Set("productType", "web-player")
	params.Set("totp", totp)
	params.Set("totpVer", version)
	params.Set("ts", strconv.FormatInt(time.Now().Unix(), 10))
	return params, nil
}

// refreshToken fetches a new access token from Spotify and stores it in the
// client.
//
// Must be called with c.mu held.
func (c *Client) refreshToken() error {
	if c.spDC == "" {
		return errors.New("sp_dc cookie is required")
	}

	params, err := c.getServerTimeParams()
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodGet, tokenURL+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Cookie", "sp_dc="+c.spDC)

	resp, err := c.tokenClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	var token tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	if token.IsAnonymous || token.AccessToken == "" {
		return errors.New("sp_dc cookie appears to be invalid")
	}

	c.token = &token
	return nil
}

// ensureToken ensures there is a valid non-expired access token, refreshing it
// if necessary. It returns the current access token string.
func (c *Client) ensureToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	nowMs := time.Now().Add(tokenExpiryBuffer).UnixMilli()
	if c.token == nil || c.token.AccessTokenExpirationTimestampMs <= nowMs {
		if err := c.refreshToken(); err != nil {
			return "", err
		}
	}

	return c.token.AccessToken, nil
}
