package subsonic_test

import (
	"context"
	//nolint:gosec // G501: verify Subsonic MD5(password+salt) wire authentication contract.
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andycarlberg/audiomoments/internal/subsonic"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func expectedMD5Token(password, salt string) string {
	//nolint:gosec // G401: Subsonic API requires MD5(password+salt) token verification.
	sum := md5.Sum([]byte(password + salt))
	return hex.EncodeToString(sum[:])
}

func writeFixture(t *testing.T, w http.ResponseWriter, name string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(loadFixture(t, name)); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func TestNewClient_Validation(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		user       string
		password   string
		httpClient *http.Client
		wantErr    bool
	}{
		{
			name:     "valid http url with trailing slash and nil httpClient",
			baseURL:  "http://localhost:4533/",
			user:     "admin",
			password: "secret",
		},
		{
			name:       "valid https url with path prefix and custom httpClient",
			baseURL:    "https://music.example.com/subsonic/",
			user:       "listener",
			password:   "secret",
			httpClient: &http.Client{},
		},
		{
			name:     "whitespace password preserved as non-empty",
			baseURL:  "http://localhost:4533",
			user:     "admin",
			password: "   ",
		},
		{
			name:     "empty url",
			baseURL:  "",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "whitespace url",
			baseURL:  "   ",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "missing scheme",
			baseURL:  "localhost:4533",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "unsupported scheme ftp",
			baseURL:  "ftp://localhost:4533",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "missing host",
			baseURL:  "http://",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "url with query string rejected",
			baseURL:  "http://localhost:4533?foo=bar",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "url with fragment rejected",
			baseURL:  "http://localhost:4533#frag",
			user:     "admin",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "empty user",
			baseURL:  "http://localhost:4533",
			user:     "",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "whitespace user",
			baseURL:  "http://localhost:4533",
			user:     "   ",
			password: "secret",
			wantErr:  true,
		},
		{
			name:     "empty password",
			baseURL:  "http://localhost:4533",
			user:     "admin",
			password: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := subsonic.NewClient(tt.baseURL, tt.user, tt.password, tt.httpClient)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("expected non-nil client")
			}
		})
	}

	t.Run("nil httpClient uses working default client", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "ping_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", nil)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("Ping with default httpClient failed: %v", err)
		}
	})
}

func TestClient_AuthContract(t *testing.T) {
	const (
		user     = "navidrome-user"
		password = "s3cr3t!Password"
	)

	var salts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/rest/ping.view" {
			t.Errorf("expected path /rest/ping.view, got %q", r.URL.Path)
		}

		q := r.URL.Query()
		if q.Has("p") {
			t.Errorf("plaintext password parameter 'p' must not be sent")
		}
		if got := q.Get("u"); got != user {
			t.Errorf("expected u=%q, got %q", user, got)
		}
		if got := q.Get("v"); got != subsonic.DefaultAPIVersion {
			t.Errorf("expected v=%q, got %q", subsonic.DefaultAPIVersion, got)
		}
		if got := q.Get("c"); got != subsonic.DefaultClientName {
			t.Errorf("expected c=%q, got %q", subsonic.DefaultClientName, got)
		}
		if got := q.Get("f"); got != "json" {
			t.Errorf("expected f='json', got %q", got)
		}

		salt := q.Get("s")
		if len(salt) < 6 {
			t.Errorf("expected salt length >= 6, got %d (%q)", len(salt), salt)
		}
		salts = append(salts, salt)

		wantToken := expectedMD5Token(password, salt)
		if got := q.Get("t"); got != wantToken {
			t.Errorf("expected token %q, got %q", wantToken, got)
		}

		writeFixture(t, w, "ping_ok.json")
	}))
	defer srv.Close()

	client, err := subsonic.NewClient(srv.URL, user, password, srv.Client())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("first Ping failed: %v", err)
	}
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("second Ping failed: %v", err)
	}

	if len(salts) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(salts))
	}
	if salts[0] == salts[1] {
		t.Errorf("expected fresh salt per request, got identical %q", salts[0])
	}
}

func TestClient_BasePathPrefix(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeFixture(t, w, "ping_ok.json")
	}))
	defer srv.Close()

	client, err := subsonic.NewClient(srv.URL+"/subsonic/", "admin", "secret", srv.Client())
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
	if gotPath != "/subsonic/rest/ping.view" {
		t.Errorf("expected path /subsonic/rest/ping.view, got %q", gotPath)
	}
}

func TestClient_Ping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "ping_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("HTTP 200 status failed with error envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "error_auth.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "wrong", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var apiErr *subsonic.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *subsonic.APIError, got %T (%v)", err, err)
		}
		if apiErr.Code != 40 {
			t.Errorf("expected error code 40, got %d", apiErr.Code)
		}
		if apiErr.Message != "Wrong username or password" {
			t.Errorf("expected message %q, got %q", "Wrong username or password", apiErr.Message)
		}
		if !strings.Contains(err.Error(), "40") || !strings.Contains(err.Error(), "Wrong username or password") {
			t.Errorf("expected error string to include code and message, got %q", err.Error())
		}
	})

	t.Run("HTTP 200 status failed without error object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"failed","version":"1.16.1"}}`))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err == nil {
			t.Fatal("expected error for status=failed without error payload")
		}
	})

	t.Run("HTTP 200 missing subsonic-response root", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err == nil {
			t.Fatal("expected error when subsonic-response root is missing")
		}
	})

	t.Run("HTTP 200 malformed JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{invalid-json`))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err == nil {
			t.Fatal("expected error for malformed JSON")
		}
	})

	t.Run("HTTP 502 non-JSON error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("<html>502 Bad Gateway</html>"))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		err = client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error for HTTP 502")
		}
		if !strings.Contains(err.Error(), "502") {
			t.Errorf("expected error to mention status 502, got %q", err.Error())
		}
	})

	t.Run("oversized response body rejected", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			oversized := make([]byte, (10<<20)+1)
			for i := range oversized {
				oversized[i] = ' '
			}
			_, _ = w.Write(oversized)
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err == nil {
			t.Fatal("expected error for oversized response body")
		}
	})

	t.Run("canceled context", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "ping_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err = client.Ping(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})

	t.Run("transport error does not leak auth query params", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "ping_ok.json")
		}))
		srvURL := srv.URL
		srv.Close()

		const (
			user     = "secret-user-name"
			password = "super-secret-password"
		)
		client, err := subsonic.NewClient(srvURL, user, password, srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected transport error against closed server")
		}
		msg := err.Error()
		for _, forbidden := range []string{user, password, "t=", "s=", "ping.view?"} {
			if strings.Contains(msg, forbidden) {
				t.Errorf("error string %q leaked %q", msg, forbidden)
			}
		}
	})

	t.Run("malformed redirect does not leak auth params", func(t *testing.T) {
		const (
			user     = "redirect-secret-user"
			password = "redirect-secret-password"
		)
		var capturedSalt, capturedToken string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			capturedSalt = q.Get("s")
			capturedToken = q.Get("t")
			w.Header().Set("Location", "http://[::1]:badport/rest/ping.view?"+r.URL.RawQuery)
			w.WriteHeader(http.StatusFound)
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, user, password, srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error for malformed redirect Location header")
		}
		if capturedSalt == "" || capturedToken == "" {
			t.Fatal("expected server to capture salt and token from request")
		}

		msg := err.Error()
		for _, forbidden := range []string{user, password, capturedSalt, capturedToken} {
			if strings.Contains(msg, forbidden) {
				t.Errorf("error string %q leaked %q", msg, forbidden)
			}
		}
		for _, expected := range []string{"u=REDACTED", "t=REDACTED", "s=REDACTED"} {
			if !strings.Contains(msg, expected) {
				t.Errorf("expected redacted placeholder %q in %q", expected, msg)
			}
		}
	})

	t.Run("nested url.Error does not leak auth params", func(t *testing.T) {
		const (
			user     = "nested-secret-user"
			password = "nested-secret-password"
		)
		var capturedSalt, capturedToken string
		httpClient := &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				q := req.URL.Query()
				capturedSalt = q.Get("s")
				capturedToken = q.Get("t")
				return nil, fmt.Errorf("proxy wrap: %w", &url.Error{
					Op:  "parse",
					URL: req.URL.String(),
					Err: errors.New("bad target " + req.URL.String()),
				})
			}),
		}

		client, err := subsonic.NewClient("https://music.example.com", user, password, httpClient)
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = client.Ping(context.Background())
		if err == nil {
			t.Fatal("expected error from nested url.Error transport")
		}
		if capturedSalt == "" || capturedToken == "" {
			t.Fatal("expected transport to capture salt and token")
		}

		msg := err.Error()
		for _, forbidden := range []string{user, password, capturedSalt, capturedToken} {
			if strings.Contains(msg, forbidden) {
				t.Errorf("error string %q leaked %q", msg, forbidden)
			}
		}
	})

	t.Run("deadline exceeded preserved", func(t *testing.T) {
		const (
			user     = "timeout-secret-user"
			password = "timeout-secret-password"
		)
		var (
			mu            sync.Mutex
			capturedSalt  string
			capturedToken string
		)
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			mu.Lock()
			capturedSalt = q.Get("s")
			capturedToken = q.Get("t")
			mu.Unlock()
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
			}
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, user, password, srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		err = client.Ping(ctx)
		srv.Close()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded, got %v", err)
		}

		mu.Lock()
		salt := capturedSalt
		token := capturedToken
		mu.Unlock()

		msg := err.Error()
		for _, forbidden := range []string{user, password, salt, token} {
			if forbidden != "" && strings.Contains(msg, forbidden) {
				t.Errorf("error string %q leaked %q", msg, forbidden)
			}
		}
	})
}

func TestClient_GetPlaylists(t *testing.T) {
	t.Run("multiple playlists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.URL.Path != "/rest/getPlaylists.view" {
				t.Errorf("expected /rest/getPlaylists.view, got %q", r.URL.Path)
			}
			writeFixture(t, w, "playlists_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		got, err := client.GetPlaylists(context.Background())
		if err != nil {
			t.Fatalf("GetPlaylists failed: %v", err)
		}

		want := []subsonic.Playlist{
			{
				ID:        "b48325a9-1d14-4e64-9a21-01a2b3c4d5e6",
				Name:      "[Moment] Upbeat: Espresso & Indie Drive",
				Comment:   "Morning upbeat station",
				Owner:     "admin",
				Public:    false,
				SongCount: 12,
				Duration:  2540,
			},
			{
				ID:        "9c2140fe-8a11-41b0-bc33-778899aabbcc",
				Name:      "Late Night Favorites",
				Comment:   "",
				Owner:     "listener",
				Public:    true,
				SongCount: 5,
				Duration:  980,
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("GetPlaylists mismatch:\n got: %+v\nwant: %+v", got, want)
		}
	})

	t.Run("empty playlists object", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "playlists_empty.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		got, err := client.GetPlaylists(context.Background())
		if err != nil {
			t.Fatalf("GetPlaylists failed: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 playlists, got %d", len(got))
		}
	})

	t.Run("omitted playlists key does not panic", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","version":"1.16.1"}}`))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		got, err := client.GetPlaylists(context.Background())
		if err != nil {
			t.Fatalf("GetPlaylists failed: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected 0 playlists, got %d", len(got))
		}
	})

	t.Run("API error envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "error_auth.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		_, err = client.GetPlaylists(context.Background())
		var apiErr *subsonic.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 40 {
			t.Fatalf("expected *subsonic.APIError code 40, got %v", err)
		}
	})
}

func TestClient_CreatePlaylist(t *testing.T) {
	t.Run("creates playlist with repeated songId in order", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.URL.Path != "/rest/createPlaylist.view" {
				t.Errorf("expected /rest/createPlaylist.view, got %q", r.URL.Path)
			}
			if r.ContentLength != 0 {
				t.Errorf("expected ContentLength 0, got %d", r.ContentLength)
			}
			if len(r.TransferEncoding) != 0 {
				t.Errorf("expected empty TransferEncoding, got %v", r.TransferEncoding)
			}
			q := r.URL.Query()
			if got := q.Get("name"); got != "[Moment] Focus: Deep Work Synth & Pulse" {
				t.Errorf("unexpected name %q", got)
			}
			wantSongs := []string{"tr-101", "tr-202"}
			if gotSongs := q["songId"]; !reflect.DeepEqual(gotSongs, wantSongs) {
				t.Errorf("expected songId=%v, got %v", wantSongs, gotSongs)
			}
			writeFixture(t, w, "create_playlist_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		got, err := client.CreatePlaylist(
			context.Background(),
			"[Moment] Focus: Deep Work Synth & Pulse",
			[]string{"tr-101", "tr-202"},
		)
		if err != nil {
			t.Fatalf("CreatePlaylist failed: %v", err)
		}

		want := &subsonic.Playlist{
			ID:        "pl-created-001",
			Name:      "[Moment] Focus: Deep Work Synth & Pulse",
			Comment:   "",
			Owner:     "admin",
			Public:    false,
			SongCount: 2,
			Duration:  420,
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CreatePlaylist mismatch:\n got: %+v\nwant: %+v", got, want)
		}
	})

	t.Run("empty songIDs omits songId parameter", func(t *testing.T) {
		for _, songs := range [][]string{nil, {}} {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Has("songId") {
					t.Errorf("expected songId parameter to be omitted, got %v", r.URL.Query()["songId"])
				}
				writeFixture(t, w, "create_playlist_ok.json")
			}))

			client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
			if err != nil {
				srv.Close()
				t.Fatalf("NewClient failed: %v", err)
			}

			if _, err := client.CreatePlaylist(context.Background(), "Empty Station", songs); err != nil {
				srv.Close()
				t.Fatalf("CreatePlaylist with empty songIDs failed: %v", err)
			}
			srv.Close()
		}
	})

	t.Run("legacy server omitting playlist object returns nil without error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","version":"1.14.0"}}`))
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		got, err := client.CreatePlaylist(context.Background(), "Legacy Playlist", []string{"tr-1"})
		if err != nil {
			t.Fatalf("CreatePlaylist failed: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil Playlist for legacy response, got %+v", got)
		}
	})

	t.Run("input validation fails before issuing HTTP request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("unexpected HTTP request on invalid CreatePlaylist input")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		tests := []struct {
			name    string
			plName  string
			songIDs []string
		}{
			{name: "empty name", plName: "", songIDs: []string{"tr-1"}},
			{name: "whitespace name", plName: "   ", songIDs: []string{"tr-1"}},
			{name: "empty songID", plName: "Valid", songIDs: []string{"tr-1", ""}},
			{name: "whitespace songID", plName: "Valid", songIDs: []string{"   "}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := client.CreatePlaylist(context.Background(), tt.plName, tt.songIDs); err == nil {
					t.Fatal("expected validation error, got nil")
				}
			})
		}
	})

	t.Run("API error envelope", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "error_auth.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		_, err = client.CreatePlaylist(context.Background(), "Station", []string{"tr-1"})
		var apiErr *subsonic.APIError
		if !errors.As(err, &apiErr) || apiErr.Code != 40 {
			t.Fatalf("expected *subsonic.APIError code 40, got %v", err)
		}
	})
}

func TestClient_UpdatePlaylist(t *testing.T) {
	t.Run("all optional fields and repeated track params", func(t *testing.T) {
		for _, pubVal := range []bool{true, false} {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/rest/updatePlaylist.view" {
					t.Errorf("expected /rest/updatePlaylist.view, got %q", r.URL.Path)
				}
				if r.ContentLength != 0 {
					t.Errorf("expected ContentLength 0, got %d", r.ContentLength)
				}
				if len(r.TransferEncoding) != 0 {
					t.Errorf("expected empty TransferEncoding, got %v", r.TransferEncoding)
				}

				q := r.URL.Query()
				if got := q.Get("playlistId"); got != "pl-123" {
					t.Errorf("expected playlistId='pl-123', got %q", got)
				}
				if got := q.Get("name"); got != "Renamed Station" {
					t.Errorf("expected name='Renamed Station', got %q", got)
				}
				if got := q.Get("comment"); got != "Updated comment" {
					t.Errorf("expected comment='Updated comment', got %q", got)
				}
				wantPub := "false"
				if pubVal {
					wantPub = "true"
				}
				if got := q.Get("public"); got != wantPub {
					t.Errorf("expected public=%q, got %q", wantPub, got)
				}
				if got := q["songIdToAdd"]; !reflect.DeepEqual(got, []string{"tr-1", "tr-2"}) {
					t.Errorf("expected songIdToAdd=[tr-1 tr-2], got %v", got)
				}
				if got := q["songIndexToRemove"]; !reflect.DeepEqual(got, []string{"0", "2"}) {
					t.Errorf("expected songIndexToRemove=[0 2], got %v", got)
				}

				writeFixture(t, w, "update_playlist_ok.json")
			}))

			client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
			if err != nil {
				srv.Close()
				t.Fatalf("NewClient failed: %v", err)
			}

			name := "Renamed Station"
			comment := "Updated comment"
			pub := pubVal
			err = client.UpdatePlaylist(context.Background(), subsonic.UpdatePlaylistParams{
				PlaylistID:          "pl-123",
				Name:                &name,
				Comment:             &comment,
				Public:              &pub,
				SongIDsToAdd:        []string{"tr-1", "tr-2"},
				SongIndexesToRemove: []int{0, 2},
			})
			srv.Close()
			if err != nil {
				t.Fatalf("UpdatePlaylist(public=%v) failed: %v", pubVal, err)
			}
		}
	})

	t.Run("nil optional pointers omitted and empty comment pointer sent", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Has("name") {
				t.Errorf("expected nil Name to be omitted, got %q", q.Get("name"))
			}
			if q.Has("public") {
				t.Errorf("expected nil Public to be omitted, got %q", q.Get("public"))
			}
			if !q.Has("comment") || q.Get("comment") != "" {
				t.Errorf("expected empty-string Comment pointer to be sent as empty string, has=%v val=%q", q.Has("comment"), q.Get("comment"))
			}
			if q.Has("songIdToAdd") {
				t.Errorf("expected empty SongIDsToAdd to be omitted, got %v", q["songIdToAdd"])
			}
			if q.Has("songIndexToRemove") {
				t.Errorf("expected empty SongIndexesToRemove to be omitted, got %v", q["songIndexToRemove"])
			}
			writeFixture(t, w, "update_playlist_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		emptyComment := ""
		err = client.UpdatePlaylist(context.Background(), subsonic.UpdatePlaylistParams{
			PlaylistID: "pl-123",
			Comment:    &emptyComment,
		})
		if err != nil {
			t.Fatalf("UpdatePlaylist failed: %v", err)
		}
	})

	t.Run("all nil optional pointers omitted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Has("name") || q.Has("comment") || q.Has("public") {
				t.Errorf("expected nil pointers to be omitted, got query %v", q)
			}
			writeFixture(t, w, "update_playlist_ok.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		if err := client.UpdatePlaylist(context.Background(), subsonic.UpdatePlaylistParams{
			PlaylistID: "pl-123",
		}); err != nil {
			t.Fatalf("UpdatePlaylist failed: %v", err)
		}
	})

	t.Run("input validation fails before issuing HTTP request", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("unexpected HTTP request on invalid UpdatePlaylist input")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		blank := "   "
		empty := ""
		tests := []struct {
			name   string
			params subsonic.UpdatePlaylistParams
		}{
			{
				name:   "empty PlaylistID",
				params: subsonic.UpdatePlaylistParams{PlaylistID: ""},
			},
			{
				name:   "whitespace PlaylistID",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "   "},
			},
			{
				name:   "non-nil empty Name",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "pl-1", Name: &empty},
			},
			{
				name:   "non-nil whitespace Name",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "pl-1", Name: &blank},
			},
			{
				name:   "negative index in SongIndexesToRemove",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "pl-1", SongIndexesToRemove: []int{0, -1}},
			},
			{
				name:   "empty songID in SongIDsToAdd",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "pl-1", SongIDsToAdd: []string{"tr-1", ""}},
			},
			{
				name:   "whitespace songID in SongIDsToAdd",
				params: subsonic.UpdatePlaylistParams{PlaylistID: "pl-1", SongIDsToAdd: []string{"   "}},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if err := client.UpdatePlaylist(context.Background(), tt.params); err == nil {
					t.Fatal("expected validation error, got nil")
				}
			})
		}
	})

	t.Run("HTTP 200 not found error code 70", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeFixture(t, w, "error_not_found.json")
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}

		err = client.UpdatePlaylist(context.Background(), subsonic.UpdatePlaylistParams{
			PlaylistID: "missing-pl",
		})
		var apiErr *subsonic.APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("expected *subsonic.APIError, got %T (%v)", err, err)
		}
		if apiErr.Code != 70 {
			t.Errorf("expected error code 70, got %d", apiErr.Code)
		}
		if apiErr.Message != "playlist not found" {
			t.Errorf("expected message %q, got %q", "playlist not found", apiErr.Message)
		}
	})
}

func TestClient_MutationsNotReplayedOnReusedConnection(t *testing.T) {
	dropConn := func(t *testing.T, w http.ResponseWriter) {
		t.Helper()
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack failed: %v", err)
			return
		}
		_ = conn.Close()
	}

	t.Run("CreatePlaylist is not replayed when reused connection drops after commit", func(t *testing.T) {
		var mutationCount atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/rest/ping.view":
				writeFixture(t, w, "ping_ok.json")
			case "/rest/createPlaylist.view":
				mutationCount.Add(1)
				dropConn(t, w)
			default:
				t.Errorf("unexpected path %q", r.URL.Path)
			}
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("initial Ping failed: %v", err)
		}

		var (
			mu             sync.Mutex
			reusedAttempts []bool
		)
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				mu.Lock()
				reusedAttempts = append(reusedAttempts, info.Reused)
				mu.Unlock()
			},
		})

		_, err = client.CreatePlaylist(ctx, "No Replay Station", []string{"tr-1"})
		if err == nil {
			t.Fatal("expected error when server dropped connection")
		}
		if got := mutationCount.Load(); got != 1 {
			t.Fatalf("expected exactly 1 createPlaylist execution, got %d", got)
		}
		mu.Lock()
		gotAttempts := append([]bool(nil), reusedAttempts...)
		mu.Unlock()
		if len(gotAttempts) != 1 || !gotAttempts[0] {
			t.Fatalf("expected 1 CreatePlaylist attempt on a reused connection, got %v", gotAttempts)
		}
	})

	t.Run("UpdatePlaylist is not replayed when reused connection drops after commit", func(t *testing.T) {
		var mutationCount atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/rest/ping.view":
				writeFixture(t, w, "ping_ok.json")
			case "/rest/updatePlaylist.view":
				mutationCount.Add(1)
				dropConn(t, w)
			default:
				t.Errorf("unexpected path %q", r.URL.Path)
			}
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("initial Ping failed: %v", err)
		}

		var (
			mu             sync.Mutex
			reusedAttempts []bool
		)
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				mu.Lock()
				reusedAttempts = append(reusedAttempts, info.Reused)
				mu.Unlock()
			},
		})

		err = client.UpdatePlaylist(ctx, subsonic.UpdatePlaylistParams{
			PlaylistID:   "pl-123",
			SongIDsToAdd: []string{"tr-1"},
		})
		if err == nil {
			t.Fatal("expected error when server dropped connection")
		}
		if got := mutationCount.Load(); got != 1 {
			t.Fatalf("expected exactly 1 updatePlaylist execution, got %d", got)
		}
		mu.Lock()
		gotAttempts := append([]bool(nil), reusedAttempts...)
		mu.Unlock()
		if len(gotAttempts) != 1 || !gotAttempts[0] {
			t.Fatalf("expected 1 UpdatePlaylist attempt on a reused connection, got %v", gotAttempts)
		}
	})

	t.Run("read-only GetPlaylists retries safely when reused connection drops", func(t *testing.T) {
		var attempts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/rest/ping.view":
				writeFixture(t, w, "ping_ok.json")
			case "/rest/getPlaylists.view":
				if attempts.Add(1) == 1 {
					dropConn(t, w)
					return
				}
				writeFixture(t, w, "playlists_ok.json")
			default:
				t.Errorf("unexpected path %q", r.URL.Path)
			}
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("initial Ping failed: %v", err)
		}

		var (
			mu             sync.Mutex
			reusedAttempts []bool
		)
		ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
			GotConn: func(info httptrace.GotConnInfo) {
				mu.Lock()
				reusedAttempts = append(reusedAttempts, info.Reused)
				mu.Unlock()
			},
		})

		playlists, err := client.GetPlaylists(ctx)
		if err != nil {
			t.Fatalf("expected GetPlaylists retry to succeed, got %v", err)
		}
		if len(playlists) != 2 {
			t.Fatalf("expected 2 playlists, got %d", len(playlists))
		}
		if got := attempts.Load(); got != 2 {
			t.Fatalf("expected 2 getPlaylists attempts, got %d", got)
		}
		mu.Lock()
		defer mu.Unlock()
		if len(reusedAttempts) != 2 || !reusedAttempts[0] || reusedAttempts[1] {
			t.Fatalf("expected first attempt reused and second fresh, got %v", reusedAttempts)
		}
	})
}

func TestClient_Redirects(t *testing.T) {
	t.Run("same-origin redirect permitted", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/rest/ping.view":
				w.Header().Set("Location", "/v2/rest/ping.view")
				w.WriteHeader(http.StatusFound)
			case "/v2/rest/ping.view":
				writeFixture(t, w, "ping_ok.json")
			default:
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
		}))
		defer srv.Close()

		client, err := subsonic.NewClient(srv.URL, "admin", "secret", srv.Client())
		if err != nil {
			t.Fatalf("NewClient failed: %v", err)
		}
		if err := client.Ping(context.Background()); err != nil {
			t.Fatalf("expected same-origin redirect to succeed, got %v", err)
		}
	})

	t.Run("cross-origin redirect rejected without leaking Referer", func(t *testing.T) {
		var foreignHits atomic.Int32
		foreignSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			foreignHits.Add(1)
			writeFixture(t, w, "ping_ok.json")
		}))
		defer foreignSrv.Close()

		originSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", foreignSrv.URL+"/capture")
			w.WriteHeader(http.StatusFound)
		}))
		defer originSrv.Close()

		t.Run("nil httpClient", func(t *testing.T) {
			client, err := subsonic.NewClient(originSrv.URL, "admin", "secret", nil)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			if err := client.Ping(context.Background()); err == nil {
				t.Fatal("expected cross-origin redirect to be rejected")
			}
		})

		t.Run("injected httpClient", func(t *testing.T) {
			injected := originSrv.Client()
			client, err := subsonic.NewClient(originSrv.URL, "admin", "secret", injected)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			if err := client.Ping(context.Background()); err == nil {
				t.Fatal("expected cross-origin redirect to be rejected")
			}
			if injected.CheckRedirect != nil {
				t.Fatal("expected caller-owned http.Client CheckRedirect not to be mutated")
			}
		})

		if got := foreignHits.Load(); got != 0 {
			t.Fatalf("expected cross-origin server to receive 0 requests, got %d", got)
		}
	})

	t.Run("injected client custom CheckRedirect preserved without mutation", func(t *testing.T) {
		t.Run("caller hop-limit policy enforced on same-origin redirects", func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/ping.view":
					w.Header().Set("Location", "/hop1")
					w.WriteHeader(http.StatusFound)
				case "/hop1":
					w.Header().Set("Location", "/hop2")
					w.WriteHeader(http.StatusFound)
				case "/hop2":
					writeFixture(t, w, "ping_ok.json")
				default:
					t.Fatalf("unexpected path %q", r.URL.Path)
				}
			}))
			defer srv.Close()

			errCallerLimit := errors.New("caller max 1 hop")
			var calls atomic.Int32
			callerClient := &http.Client{
				Transport: srv.Client().Transport,
				CheckRedirect: func(_ *http.Request, via []*http.Request) error {
					calls.Add(1)
					if len(via) > 1 {
						return errCallerLimit
					}
					return nil
				},
			}

			client, err := subsonic.NewClient(srv.URL, "admin", "secret", callerClient)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}
			err = client.Ping(context.Background())
			if !errors.Is(err, errCallerLimit) {
				t.Fatalf("expected caller error %v, got %v", errCallerLimit, err)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("expected caller CheckRedirect called twice, got %d", got)
			}
		})

		t.Run("cross-origin Referer stripped before caller CheckRedirect and caller client unmutated", func(t *testing.T) {
			var foreignHits atomic.Int32
			foreignSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				foreignHits.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer foreignSrv.Close()

			originSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", foreignSrv.URL+"/capture")
				w.WriteHeader(http.StatusFound)
			}))
			defer originSrv.Close()

			var (
				mu          sync.Mutex
				seenReferer []string
			)
			callerClient := &http.Client{
				Transport: originSrv.Client().Transport,
				CheckRedirect: func(req *http.Request, _ []*http.Request) error {
					mu.Lock()
					seenReferer = append(seenReferer, req.Header.Get("Referer"))
					mu.Unlock()
					return http.ErrUseLastResponse
				},
			}

			client, err := subsonic.NewClient(originSrv.URL, "admin", "secret", callerClient)
			if err != nil {
				t.Fatalf("NewClient failed: %v", err)
			}

			err = client.Ping(context.Background())
			if err == nil || !strings.Contains(err.Error(), "302") {
				t.Fatalf("expected unexpected HTTP status 302 from ErrUseLastResponse, got %v", err)
			}
			if got := foreignHits.Load(); got != 0 {
				t.Fatalf("expected foreign server to receive 0 requests, got %d", got)
			}

			// Verify caller's own client was not wrapped/mutated by checking direct call still sees Referer.
			directReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, originSrv.URL+"/rest/ping.view?u=caller", nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext failed: %v", err)
			}
			directResp, err := callerClient.Do(directReq)
			if err != nil {
				t.Fatalf("callerClient.Do failed: %v", err)
			}
			_ = directResp.Body.Close()

			mu.Lock()
			defer mu.Unlock()
			if len(seenReferer) != 2 {
				t.Fatalf("expected 2 CheckRedirect observations, got %d", len(seenReferer))
			}
			if seenReferer[0] != "" {
				t.Fatalf("expected empty Referer on cross-origin redirect via subsonic.Client, got %q", seenReferer[0])
			}
			if !strings.Contains(seenReferer[1], "u=caller") {
				t.Fatalf("expected callerClient direct call to retain stdlib Referer (unmutated), got %q", seenReferer[1])
			}
		})
	})
}
