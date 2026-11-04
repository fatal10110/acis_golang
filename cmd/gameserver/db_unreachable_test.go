package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// unreachableDBBootEnv names the config directory a re-executed test binary
// boots the game server from.
const unreachableDBBootEnv = "ACIS_TEST_UNREACHABLE_DB_BOOT"

// TestGameServerRefusesToBootWithUnreachableDatabase boots the real game
// server graph against a database address nothing listens on. Booting
// without the database would start from an empty used-id set and empty
// clan, castle and hero tables, so the process must refuse: it exits
// non-zero, logs why, and never opens the game client listener.
//
// The boot runs in a child process, so its exit status is the one the binary
// reports and nothing it started outlives it. The parent holds the client
// port for the whole run: a bind attempted before the database refusal would
// fail on that port, and the child would then report the bind instead.
func TestGameServerRefusesToBootWithUnreachableDatabase(t *testing.T) {
	if dir := os.Getenv(unreachableDBBootEnv); dir != "" {
		newGameServerApp(unreachableDBPaths(dir)).Run()
		t.Fatal("game server started without its database")
	}

	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold client port: %v", err)
	}
	defer held.Close()
	port := held.Addr().(*net.TCPAddr).Port

	dir := t.TempDir()
	writeUnreachableDBConfig(t, dir, port)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGameServerRefusesToBootWithUnreachableDatabase$")
	cmd.Env = append(os.Environ(), unreachableDBBootEnv+"="+dir)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("game server still running after %v with an unreachable database:\n%s", 2*time.Minute, out)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Fatalf("boot with an unreachable database: err = %v, want a non-zero exit\n%s", err, out)
	}
	log := string(out)
	reason := refusalLine(log)
	if reason == "" {
		t.Fatalf("boot output does not name the unreachable database:\n%s", log)
	}
	t.Logf("refusal: %s", reason)
	if strings.Contains(log, "listen for game clients") {
		t.Fatalf("boot reached the game client listener before refusing:\n%s", log)
	}
}

func unreachableDBPaths(dir string) gameServerPaths {
	return gameServerPaths{
		ConfigPath:        filepath.Join(dir, "server.properties"),
		LoggingPath:       filepath.Join(dir, "logging.properties"),
		PlayersConfigPath: filepath.Join(dir, "players.properties"),
		HexIDPath:         filepath.Join(dir, "hexid.txt"),
		GeoConfigPath:     filepath.Join(dir, "geoengine.properties"),
		NpcsConfigPath:    filepath.Join(dir, "npcs.properties"),
		ClansConfigPath:   filepath.Join(dir, "clans.properties"),
		EventsConfigPath:  filepath.Join(dir, "events.properties"),
		SiegeConfigPath:   filepath.Join(dir, "siege.properties"),
		DataRoot:          filepath.Join(dir, "datapack"),
		LogRoot:           filepath.Join(dir, "log"),
	}
}

// writeUnreachableDBConfig writes a config directory whose database URL
// points at a port nothing listens on and whose client listener is port.
func writeUnreachableDBConfig(t *testing.T, dir string, port int) {
	t.Helper()
	files := map[string]string{
		"server.properties": "URL = jdbc:mariadb://127.0.0.1:1/acis\nLogin = acis\nPassword = acis\n" +
			"GameserverHostname = 127.0.0.1\nGameserverPort = " + strconv.Itoa(port) + "\n",
	}
	files["hexid.txt"] = "ServerID = 1\nHexID = 0a\n"
	for _, name := range []string{
		"logging.properties", "players.properties", "geoengine.properties",
		"npcs.properties", "clans.properties", "events.properties", "siege.properties",
	} {
		files[name] = ""
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range []string{"datapack", "log"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// refusalLine returns the first line of log reporting the database
// unreachable at the configured address, or "".
func refusalLine(log string) string {
	for line := range strings.Lines(log) {
		if strings.Contains(line, "reach database") && strings.Contains(line, "127.0.0.1:1") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
