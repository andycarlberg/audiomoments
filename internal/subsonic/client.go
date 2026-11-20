// Package subsonic implements a lightweight OpenSubsonic and Subsonic REST client.
package subsonic

import (
	"context"
	//nolint:gosec // G501: Subsonic API v1.13+ mandates MD5(password+salt) token authentication.
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var authQueryParamPattern = regexp.MustCompile(`(?i)([?&])(u|t|s|p)=[^&\s"'\\]*`)

// Default protocol, client, and HTTP transport values.
const (
	DefaultAPIVersion = "1.16.1"
	DefaultClientName = "audiomoments"
	DefaultTimeout    = 15 * time.Second
)

const (
	saltByteLength   = 8
	maxResponseBytes = 10 << 20
)

// APIError represents an application-level error returned inside a Subsonic response envelope.
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("subsonic error %d: %s", e.Code, e.Message)
}

// Playlist describes a playlist returned by the OpenSubsonic REST API.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Comment   string `json:"comment"`
	Owner     string `json:"owner"`
	Public    bool   `json:"public"`
	SongCount int    `json:"songCount"`
	Duration  int    `json:"duration"`
}

// UpdatePlaylistParams configures an updatePlaylist request.
// Nil pointer fields leave the corresponding playlist metadata unchanged on the server.
type UpdatePlaylistParams struct {
	PlaylistID          string
	Name                *string
	Comment             *string
	Public              *bool
	SongIDsToAdd        []string
	SongIndexesToRemove []int
}

// Client communicates with an OpenSubsonic- or Subsonic-compatible music server over HTTP.
type Client struct {
	baseURL    *url.URL
	user       string
	password   string
	httpClient *http.Client
}

type responseEnvelope struct {
	SubsonicResponse *subsonicResponse `json:"subsonic-response"`
}

type subsonicResponse struct {
	Status    string            `json:"status"`
	Error     *APIError         `json:"error"`
	Playlists *playlistsPayload `json:"playlists"`
	Playlist  *Playlist         `json:"playlist"`
}

type playlistsPayload struct {
	Playlist []Playlist `json:"playlist"`
}

// NewClient constructs a validated Subsonic REST client.
// When httpClient is nil, a client with DefaultTimeout is created.
func NewClient(baseURL, user, password string, httpClient *http.Client) (*Client, error) {
	rawURL := strings.TrimSpace(baseURL)
	if rawURL == "" {
		return nil, errors.New("subsonic base URL is required")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid subsonic base URL %q: must be a valid http or https URL", rawURL)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid subsonic base URL %q: query strings and fragments are not allowed", rawURL)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	trimmedUser := strings.TrimSpace(user)
	if trimmedUser == "" {
		return nil, errors.New("subsonic user is required")
	}
	if password == "" {
		return nil, errors.New("subsonic password is required")
	}

	return &Client{
		baseURL:    parsed,
		user:       trimmedUser,
		password:   password,
		httpClient: wrapHTTPClient(parsed, httpClient),
	}, nil
}

// Ping verifies server connectivity and authentication credentials.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "ping", nil, true)
	return err
}

// GetPlaylists retrieves all playlists visible to the authenticated user.
func (c *Client) GetPlaylists(ctx context.Context) ([]Playlist, error) {
	resp, err := c.do(ctx, "getPlaylists", nil, true)
	if err != nil {
		return nil, err
	}
	if resp.Playlists == nil || len(resp.Playlists.Playlist) == 0 {
		return []Playlist{}, nil
	}
	return resp.Playlists.Playlist, nil
}

// CreatePlaylist creates a new playlist on the server with the given name and ordered song IDs.
// Servers running legacy Subsonic <= 1.14.0 may omit the created playlist object and return (nil, nil).
func (c *Client) CreatePlaylist(ctx context.Context, name string, songIDs []string) (*Playlist, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("subsonic createPlaylist: playlist name is required")
	}

	q := url.Values{}
	q.Set("name", name)
	for i, songID := range songIDs {
		if strings.TrimSpace(songID) == "" {
			return nil, fmt.Errorf("subsonic createPlaylist: songIDs[%d] must be non-blank", i)
		}
		q.Add("songId", songID)
	}

	resp, err := c.do(ctx, "createPlaylist", q, false)
	if err != nil {
		return nil, err
	}
	return resp.Playlist, nil
}

// UpdatePlaylist updates metadata or tracks for an existing playlist.
func (c *Client) UpdatePlaylist(ctx context.Context, params UpdatePlaylistParams) error {
	if strings.TrimSpace(params.PlaylistID) == "" {
		return errors.New("subsonic updatePlaylist: playlist ID is required")
	}
	if params.Name != nil && strings.TrimSpace(*params.Name) == "" {
		return errors.New("subsonic updatePlaylist: playlist name must be non-blank when set")
	}

	q := url.Values{}
	q.Set("playlistId", params.PlaylistID)
	if params.Name != nil {
		q.Set("name", *params.Name)
	}
	if params.Comment != nil {
		q.Set("comment", *params.Comment)
	}
	if params.Public != nil {
		q.Set("public", strconv.FormatBool(*params.Public))
	}

	for i, songID := range params.SongIDsToAdd {
		if strings.TrimSpace(songID) == "" {
			return fmt.Errorf("subsonic updatePlaylist: SongIDsToAdd[%d] must be non-blank", i)
		}
		q.Add("songIdToAdd", songID)
	}
	for i, idx := range params.SongIndexesToRemove {
		if idx < 0 {
			return fmt.Errorf("subsonic updatePlaylist: SongIndexesToRemove[%d] must be >= 0, got %d", i, idx)
		}
		q.Add("songIndexToRemove", strconv.Itoa(idx))
	}

	_, err := c.do(ctx, "updatePlaylist", q, false)
	return err
}

func (c *Client) do(ctx context.Context, endpoint string, params url.Values, idempotent bool) (*subsonicResponse, error) {
	salt, err := generateSalt()
	if err != nil {
		return nil, fmt.Errorf("subsonic %s: generate auth salt: %w", endpoint, err)
	}
	token := computeToken(c.password, salt)

	q := make(url.Values, len(params)+6)
	for k, vals := range params {
		copied := make([]string, len(vals))
		copy(copied, vals)
		q[k] = copied
	}
	q.Set("u", c.user)
	q.Set("t", token)
	q.Set("s", salt)
	q.Set("v", DefaultAPIVersion)
	q.Set("c", DefaultClientName)
	q.Set("f", "json")

	reqURL := *c.baseURL
	reqURL.Path = reqURL.Path + "/rest/" + endpoint + ".view"
	reqURL.RawQuery = q.Encode()

	var body io.Reader
	if !idempotent {
		// Non-nil body without GetBody marks Request.isReplayable() false so net/http
		// does not automatically retry mutations after an ambiguous connection drop.
		body = nonReplayableEmptyBody{}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), body)
	if err != nil {
		return nil, scrubURLError(endpoint, err, token, salt)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, scrubURLError(endpoint, err, token, salt)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subsonic %s: unexpected HTTP status %d", endpoint, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("subsonic %s: read response: %w", endpoint, err)
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("subsonic %s: response exceeds %d bytes", endpoint, maxResponseBytes)
	}

	var env responseEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("subsonic %s: decode JSON response: %w", endpoint, err)
	}
	if env.SubsonicResponse == nil {
		return nil, fmt.Errorf("subsonic %s: missing subsonic-response envelope", endpoint)
	}
	if env.SubsonicResponse.Error != nil {
		return nil, fmt.Errorf("subsonic %s: %w", endpoint, env.SubsonicResponse.Error)
	}
	if env.SubsonicResponse.Status != "ok" {
		return nil, fmt.Errorf("subsonic %s: unexpected status %q", endpoint, env.SubsonicResponse.Status)
	}

	return env.SubsonicResponse, nil
}

type nonReplayableEmptyBody struct{}

func (nonReplayableEmptyBody) Read([]byte) (int, error) { return 0, io.EOF }
func (nonReplayableEmptyBody) Close() error             { return nil }

func isNonReplayable(req *http.Request) bool {
	_, ok := req.Body.(nonReplayableEmptyBody)
	return ok
}

type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

func wrapHTTPClient(baseURL *url.URL, httpClient *http.Client) *http.Client {
	var client http.Client
	if httpClient == nil {
		client = http.Client{Timeout: DefaultTimeout}
	} else {
		client = *httpClient
	}

	prevCheckRedirect := client.CheckRedirect
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !sameOrigin(baseURL, req.URL) {
			req.Header.Del("Referer")
			if prevCheckRedirect != nil {
				if err := prevCheckRedirect(req, via); err != nil {
					return err
				}
			}
			return errors.New("cross-origin redirect is not allowed")
		}

		if len(via) > 0 && isNonReplayable(via[0]) {
			req.Body = nonReplayableEmptyBody{}
			req.GetBody = nil
		}

		if prevCheckRedirect != nil {
			return prevCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	if !strings.EqualFold(a.Scheme, b.Scheme) {
		return false
	}
	if !strings.EqualFold(a.Hostname(), b.Hostname()) {
		return false
	}
	return effectivePort(a) == effectivePort(b)
}

func effectivePort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch strings.ToLower(u.Scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}

func scrubURLError(endpoint string, err error, token, salt string) error {
	inner := err
	for {
		var urlErr *url.Error
		if !errors.As(inner, &urlErr) || urlErr.Err == nil {
			break
		}
		inner = urlErr.Err
	}

	redacted := redactAuthParams(inner.Error(), token, salt)
	if redacted != inner.Error() {
		return &redactedError{
			msg: fmt.Sprintf("subsonic %s: %s", endpoint, redacted),
			err: inner,
		}
	}
	return fmt.Errorf("subsonic %s: %w", endpoint, inner)
}

func redactAuthParams(msg, token, salt string) string {
	redacted := authQueryParamPattern.ReplaceAllString(msg, "${1}${2}=REDACTED")
	if token != "" {
		redacted = strings.ReplaceAll(redacted, token, "REDACTED")
	}
	if salt != "" {
		redacted = strings.ReplaceAll(redacted, salt, "REDACTED")
	}
	return redacted
}

func generateSalt() (string, error) {
	buf := make([]byte, saltByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func computeToken(password, salt string) string {
	//nolint:gosec // G401: Subsonic API v1.13+ mandates MD5(password+salt) token authentication.
	sum := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(sum[:])
}
