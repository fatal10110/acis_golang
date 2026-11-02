package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func artifactZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

// fakeGitHub serves one run (42) and its artifact, whose download redirects to
// a separate storage host like the real API does.
func fakeGitHub(t *testing.T, run map[string]string, zipBody []byte) *httptest.Server {
	t.Helper()
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("job token leaked to the storage host")
		}
		w.Write(zipBody)
	}))
	t.Cleanup(storage.Close)

	mux := http.NewServeMux()
	var api *httptest.Server
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer job-token" {
				http.Error(w, "bad credentials", http.StatusUnauthorized)
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /repos/o/r/actions/runs/42", authed(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(run)
	}))
	mux.HandleFunc("GET /repos/o/r/actions/runs/42/artifacts", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != artifactName {
			t.Errorf("artifact name = %q", r.URL.Query().Get("name"))
		}
		json.NewEncoder(w).Encode(map[string]any{"artifacts": []map[string]any{
			{"archive_download_url": api.URL + "/zip", "expired": false},
		}})
	}))
	mux.HandleFunc("GET /zip", authed(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, storage.URL+"/blob", http.StatusFound)
	}))
	api = httptest.NewServer(mux)
	t.Cleanup(api.Close)
	return api
}

func TestDeploy(t *testing.T) {
	const old = "\x7fELF old"
	goodRun := map[string]string{"path": deployWorkflow, "head_branch": "main", "head_sha": "abc123"}
	goodZip := map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF ls"}
	tests := []struct {
		name     string
		token    string
		ghToken  string
		run      map[string]string
		zip      map[string]string
		code     int
		swapped  bool
		restarts []string
	}{
		{
			name:     "deploy run on main swaps and restarts in order",
			token:    "secret",
			ghToken:  "job-token",
			run:      goodRun,
			zip:      goodZip,
			code:     http.StatusOK,
			swapped:  true,
			restarts: services,
		},
		{
			name:    "wrong deploy token",
			token:   "nope",
			ghToken: "job-token",
			run:     goodRun,
			zip:     goodZip,
			code:    http.StatusUnauthorized,
		},
		{
			name:    "rejected job token",
			token:   "secret",
			ghToken: "stale",
			run:     goodRun,
			zip:     goodZip,
			code:    http.StatusBadGateway,
		},
		{
			name:    "run from another workflow",
			token:   "secret",
			ghToken: "job-token",
			run:     map[string]string{"path": ".github/workflows/go.yml", "head_branch": "main"},
			zip:     goodZip,
			code:    http.StatusBadGateway,
		},
		{
			name:    "run on another branch",
			token:   "secret",
			ghToken: "job-token",
			run:     map[string]string{"path": deployWorkflow, "head_branch": "feature"},
			zip:     goodZip,
			code:    http.StatusBadGateway,
		},
		{
			name:    "artifact missing a binary",
			token:   "secret",
			ghToken: "job-token",
			run:     goodRun,
			zip:     map[string]string{"gameserver": "\x7fELF gs"},
			code:    http.StatusBadGateway,
		},
		{
			name:    "artifact holds a non-ELF file",
			token:   "secret",
			ghToken: "job-token",
			run:     goodRun,
			zip:     map[string]string{"gameserver": "\x7fELF gs", "loginserver": "#!/bin/sh"},
			code:    http.StatusBadGateway,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range binaries {
				os.WriteFile(filepath.Join(dir, b), []byte(old), 0o644)
			}
			api := fakeGitHub(t, tc.run, artifactZip(t, tc.zip))
			var restarted []string
			h := newHandler(config{
				token:  "secret",
				api:    api.URL,
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
			req.Header.Set("X-GitHub-Token", tc.ghToken)
			req.Header.Set("X-Deploy-Run", "42")
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
			if left, _ := filepath.Glob(filepath.Join(dir, "*.zip")); len(left) > 0 {
				t.Errorf("downloaded zip left behind: %v", left)
			}
		})
	}
}
