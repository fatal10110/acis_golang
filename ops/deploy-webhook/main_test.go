package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func deployRequest(t *testing.T, token string, parts map[string]string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, content := range parts {
		fw, err := mw.CreateFormFile(name, name)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write([]byte(content))
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/deploy", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestDeploy(t *testing.T) {
	const old = "\x7fELF old"
	tests := []struct {
		name     string
		token    string
		parts    map[string]string
		code     int
		swapped  bool
		restarts []string
	}{
		{
			name:     "both binaries swap and restart in order",
			token:    "secret",
			parts:    map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF ls"},
			code:     http.StatusOK,
			swapped:  true,
			restarts: services,
		},
		{
			name:  "wrong token",
			token: "nope",
			parts: map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF ls"},
			code:  http.StatusUnauthorized,
		},
		{
			name:  "missing binary",
			token: "secret",
			parts: map[string]string{"gameserver": "\x7fELF gs"},
			code:  http.StatusBadRequest,
		},
		{
			name:  "not an ELF",
			token: "secret",
			parts: map[string]string{"gameserver": "#!/bin/sh", "loginserver": "\x7fELF ls"},
			code:  http.StatusBadRequest,
		},
		{
			name:  "unexpected field",
			token: "secret",
			parts: map[string]string{"gameserver": "\x7fELF gs", "loginserver": "\x7fELF ls", "../x": "\x7fELF"},
			code:  http.StatusBadRequest,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, b := range binaries {
				os.WriteFile(filepath.Join(dir, b), []byte(old), 0o755)
			}
			var restarted []string
			h := newHandler("secret", dir, func(unit string) error {
				restarted = append(restarted, unit)
				return nil
			})

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, deployRequest(t, tc.token, tc.parts))

			if rec.Code != tc.code {
				t.Fatalf("code = %d, want %d (%s)", rec.Code, tc.code, rec.Body)
			}
			if !slices.Equal(restarted, tc.restarts) {
				t.Errorf("restarts = %v, want %v", restarted, tc.restarts)
			}
			for _, b := range binaries {
				got, _ := os.ReadFile(filepath.Join(dir, b))
				if swapped := string(got) != old; swapped != tc.swapped {
					t.Errorf("%s swapped = %v, want %v", b, swapped, tc.swapped)
				}
			}
			if left, _ := filepath.Glob(filepath.Join(dir, "*.new")); len(left) > 0 {
				t.Errorf("staged files left behind: %v", left)
			}
		})
	}
}
