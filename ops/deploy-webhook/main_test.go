package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sumsFor renders sha256sum output for the given assets.
func sumsFor(assets map[string]string) string {
	var b strings.Builder
	for name, content := range assets {
		fmt.Fprintf(&b, "%s  %s\n", sha(content), name)
	}
	return b.String()
}

// fakeReleases serves the assets of release "deploy-abc" the way GitHub does:
// the download URL redirects to a separate storage host.
func fakeReleases(t *testing.T, assets map[string]string) *httptest.Server {
	t.Helper()
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content, ok := assets[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(content))
	}))
	t.Cleanup(storage.Close)
	web := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, "/o/r/releases/download/deploy-abc/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, storage.URL+"/"+name, http.StatusFound)
	}))
	t.Cleanup(web.Close)
	return web
}

func TestDeploy(t *testing.T) {
	const old = "\x7fELF old"
	good := map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF ls"}
	withSums := func(bins map[string]string, sums string) map[string]string {
		assets := map[string]string{sumsAsset: sums}
		maps.Copy(assets, bins)
		return assets
	}
	tests := []struct {
		name     string
		token    string
		tag      string
		assets   map[string]string
		code     int
		swapped  bool
		restarts []string
	}{
		{
			name:     "verified release swaps and restarts in order",
			token:    "secret",
			tag:      "deploy-abc",
			assets:   withSums(good, sumsFor(good)),
			code:     http.StatusOK,
			swapped:  true,
			restarts: services,
		},
		{
			name:   "wrong deploy token",
			token:  "nope",
			tag:    "deploy-abc",
			assets: withSums(good, sumsFor(good)),
			code:   http.StatusUnauthorized,
		},
		{
			name:   "tag with a path in it",
			token:  "secret",
			tag:    "../../evil/releases/download/x",
			assets: withSums(good, sumsFor(good)),
			code:   http.StatusBadRequest,
		},
		{
			name:   "unknown release",
			token:  "secret",
			tag:    "deploy-zzz",
			assets: withSums(good, sumsFor(good)),
			code:   http.StatusBadGateway,
		},
		{
			name:   "binary does not match its checksum",
			token:  "secret",
			tag:    "deploy-abc",
			assets: withSums(map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF tampered"}, sumsFor(good)),
			code:   http.StatusBadGateway,
		},
		{
			name:   "release missing a binary",
			token:  "secret",
			tag:    "deploy-abc",
			assets: withSums(map[string]string{"gameserver": "\x7fELF gs"}, sumsFor(good)),
			code:   http.StatusBadGateway,
		},
		{
			name:   "checksums missing a binary",
			token:  "secret",
			tag:    "deploy-abc",
			assets: withSums(good, sumsFor(map[string]string{"gameserver": "\x7fELF gs"})),
			code:   http.StatusBadGateway,
		},
		{
			name:   "non-ELF asset with a matching checksum",
			token:  "secret",
			tag:    "deploy-abc",
			assets: withSums(map[string]string{"gameserver": "\x7fELF gs", "loginserver": "#!/bin/sh"}, sumsFor(map[string]string{"gameserver": "\x7fELF gs", "loginserver": "#!/bin/sh"})),
			code:   http.StatusBadGateway,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range binaries {
				os.WriteFile(filepath.Join(dir, b), []byte(old), 0o644)
			}
			var restarted []string
			h := newHandler(config{
				token:  "secret",
				base:   fakeReleases(t, tc.assets).URL,
				repo:   "o/r",
				binDir: dir,
				client: http.DefaultClient,
				restart: func(unit string) error {
					restarted = append(restarted, unit)
					return nil
				},
			})

			req := httptest.NewRequest(http.MethodPost, "/deploy", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.Header.Set("X-Deploy-Tag", tc.tag)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.code {
				t.Fatalf("code = %d, want %d (%s)", rec.Code, tc.code, rec.Body)
			}
			if !slices.Equal(restarted, tc.restarts) {
				t.Errorf("restarts = %v, want %v", restarted, tc.restarts)
			}
			for _, b := range binaries {
				path := filepath.Join(dir, b)
				got, _ := os.ReadFile(path)
				if swapped := string(got) != old; swapped != tc.swapped {
					t.Errorf("%s swapped = %v, want %v", b, swapped, tc.swapped)
				}
				if fi, _ := os.Stat(path); tc.swapped && fi.Mode().Perm() != 0o755 {
					t.Errorf("%s mode = %v, want 0755", b, fi.Mode().Perm())
				}
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*.new")); len(left) > 0 {
				t.Errorf("staged files left behind: %v", left)
			}
		})
	}
}
