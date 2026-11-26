package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/link"
	"github.com/fatal10110/acis_golang/internal/loginserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/loginserver/model"
)

var registerNow = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

// registerOnce runs the non-interactive -register path, the way one
// `gsregister -register id -out path` invocation does.
func registerOnce(t *testing.T, store *sql.GameServerStore, id int, path string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := register(context.Background(), &out, loadTestNames(t), store, id, path, registerNow)
	return out.String(), err
}

// hexIDFile reads a hexid file the way the game server does: ServerID and
// HexID properties.
func hexIDFile(t *testing.T, path string) (id int, hexText string) {
	t.Helper()
	fileID, key, err := link.ReadHexIDFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return fileID, link.HexKeyText(key)
}

func TestRegisterFlagRegistersAndRerunIsNoOp(t *testing.T) {
	store := newIntegrationStore(t)
	path := filepath.Join(t.TempDir(), "config", "hexid.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := registerOnce(t, store, 1, path)
	if err != nil {
		t.Fatalf("first register: %v\n%s", err, out)
	}
	row, err := store.GameServer(context.Background(), 1)
	if err != nil {
		t.Fatalf("GameServer(1) after register: %v", err)
	}
	// The stored key is a signed big-integer, so it reads back in its minimal
	// two's-complement form: shorter than authKeyLen when the random key
	// starts with zero bytes. Compare the normalized form, not the raw length.
	if n := len(row.HexID); n == 0 || n > authKeyLen {
		t.Fatalf("registered key is %d bytes, want 1..%d", n, authKeyLen)
	}
	if reparsed, err := link.ParseHexKey(link.HexKeyText(row.HexID)); err != nil || !bytes.Equal(reparsed, row.HexID) {
		t.Fatalf("registered key %x is not in normalized form: reparsed %x, err %v", row.HexID, reparsed, err)
	}
	id, hexText := hexIDFile(t, path)
	if id != 1 || hexText != link.HexKeyText(row.HexID) {
		t.Fatalf("hexid file = ServerID %d HexID %s, want 1 %s", id, hexText, link.HexKeyText(row.HexID))
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	out, err = registerOnce(t, store, 1, path)
	if err != nil {
		t.Fatalf("rerun on a registered database: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already registered") {
		t.Errorf("rerun output = %q, want it to report the existing registration", out)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("rerun rewrote the hexid file:\nbefore %q\nafter  %q", before, after)
	}
	again, err := store.GameServer(context.Background(), 1)
	if err != nil || !bytes.Equal(again.HexID, row.HexID) {
		t.Errorf("rerun changed the registered key: %v", err)
	}
}

func TestRegisterFlagRewritesMissingFileFromDatabase(t *testing.T) {
	store := newIntegrationStore(t)
	key := []byte{0x7f, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	if err := store.CreateGameServer(context.Background(), model.NewGameServer(1, key, "")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "hexid.txt")

	if out, err := registerOnce(t, store, 1, path); err != nil {
		t.Fatalf("register: %v\n%s", err, out)
	}
	id, hexText := hexIDFile(t, path)
	if id != 1 || hexText != link.HexKeyText(key) {
		t.Fatalf("hexid file = ServerID %d HexID %s, want the stored key %s", id, hexText, link.HexKeyText(key))
	}
}

func TestRegisterFlagRegistersExistingFileKey(t *testing.T) {
	store := newIntegrationStore(t)
	path := filepath.Join(t.TempDir(), "hexid.txt")
	key := []byte{0x11, 0x22, 0x33}
	if err := link.WriteHexIDFile(path, 1, key, registerNow); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if out, err := registerOnce(t, store, 1, path); err != nil {
		t.Fatalf("register over a recreated database: %v\n%s", err, out)
	}
	row, err := store.GameServer(context.Background(), 1)
	if err != nil {
		t.Fatalf("GameServer(1): %v", err)
	}
	if link.HexKeyText(row.HexID) != link.HexKeyText(key) {
		t.Errorf("registered key %s, want the file's %s", link.HexKeyText(row.HexID), link.HexKeyText(key))
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("register rewrote the hexid file")
	}
}

func TestRegisterFlagRejectsDifferentHexID(t *testing.T) {
	store := newIntegrationStore(t)
	stored := []byte{0x01, 0x02}
	if err := store.CreateGameServer(context.Background(), model.NewGameServer(1, stored, "")); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	for _, tc := range []struct {
		name string
		id   int
		key  []byte
	}{
		{"other key", 1, []byte{0x03, 0x04}},
		{"other server id", 2, stored},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".txt")
			if err := link.WriteHexIDFile(path, tc.id, tc.key, registerNow); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			out, err := registerOnce(t, store, 1, path)
			if err == nil || !strings.Contains(err.Error(), "differs") {
				t.Fatalf("register over a mismatched file: err %v, want a differs error\n%s", err, out)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("mismatch rewrote the hexid file")
			}
			row, err := store.GameServer(context.Background(), 1)
			if err != nil || !bytes.Equal(row.HexID, stored) {
				t.Errorf("mismatch changed the registration: %v", err)
			}
		})
	}
}

func TestRegisterFlagRejectsFileForAnotherUnregisteredServer(t *testing.T) {
	store := newIntegrationStore(t)
	path := filepath.Join(t.TempDir(), "hexid.txt")
	if err := link.WriteHexIDFile(path, 2, []byte{0x05}, registerNow); err != nil {
		t.Fatal(err)
	}
	if _, err := registerOnce(t, store, 1, path); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("register 1 over server 2's file: err %v, want a differs error", err)
	}
	if _, err := store.GameServer(context.Background(), 1); !errors.Is(err, sql.ErrGameServerNotFound) {
		t.Fatalf("GameServer(1): err %v, want ErrGameServerNotFound", err)
	}
}

func TestRegisterFlagRejectsUnlistedID(t *testing.T) {
	store := newIntegrationStore(t)
	path := filepath.Join(t.TempDir(), "hexid.txt")
	if _, err := registerOnce(t, store, 9, path); err == nil || !strings.Contains(err.Error(), "no name for server id 9") {
		t.Fatalf("register 9: err %v, want the no-name rejection", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("unlisted id wrote %s: %v", path, err)
	}
	if _, err := store.GameServer(context.Background(), 9); !errors.Is(err, sql.ErrGameServerNotFound) {
		t.Fatalf("GameServer(9): err %v, want ErrGameServerNotFound", err)
	}
}
