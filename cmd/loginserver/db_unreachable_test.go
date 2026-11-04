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
// boots the login server from.
const unreachableDBBootEnv = "ACIS_TEST_UNREACHABLE_DB_BOOT"

// TestLoginServerRefusesToBootWithUnreachableDatabase boots the real login
// server graph against a database address nothing listens on. Without the
// database there are no accounts and no registered game servers to accept,
// so the process must refuse: it exits non-zero, logs why, and never opens
// its client or game server listener.
//
// The boot runs in a child process, so its exit status is the one the binary
// reports and nothing it started outlives it. The parent holds both ports for
// the whole run: a bind attempted before the database refusal would fail on
// them, and the child would then report the bind instead.
func TestLoginServerRefusesToBootWithUnreachableDatabase(t *testing.T) {
	if dir := os.Getenv(unreachableDBBootEnv); dir != "" {
		newLoginServerApp(unreachableDBPaths(dir)).Run()
		t.Fatal("login server started without its database")
	}

	clientPort, gamePort := holdPort(t), holdPort(t)
	dir := t.TempDir()
	writeUnreachableDBConfig(t, dir, clientPort, gamePort)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoginServerRefusesToBootWithUnreachableDatabase$")
	cmd.Env = append(os.Environ(), unreachableDBBootEnv+"="+dir)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("login server still running after %v with an unreachable database:\n%s", 2*time.Minute, out)
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
	if strings.Contains(log, "listen for") || strings.Contains(log, "listening for gameservers") {
		t.Fatalf("boot reached a listener before refusing:\n%s", log)
	}
}

// holdPort binds a loopback port for the rest of the test and returns it.
func holdPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold port: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

func unreachableDBPaths(dir string) loginServerPaths {
	return loginServerPaths{
		ConfigPath:      filepath.Join(dir, "loginserver.properties"),
		LoggingPath:     filepath.Join(dir, "logging.properties"),
		ServerNamesPath: filepath.Join(dir, "serverNames.xml"),
		BannedIPsPath:   filepath.Join(dir, "banned_ips.properties"),
		LogRoot:         filepath.Join(dir, "log"),
	}
}

// writeUnreachableDBConfig writes a config directory whose database URL
// points at a port nothing listens on and whose listeners are the two ports.
func writeUnreachableDBConfig(t *testing.T, dir string, clientPort, gamePort int) {
	t.Helper()
	files := map[string]string{
		"loginserver.properties": "URL = jdbc:mariadb://127.0.0.1:1/acis\nLogin = acis\nPassword = acis\n" +
			"LoginserverHostname = 127.0.0.1\nLoginserverPort = " + strconv.Itoa(clientPort) + "\n" +
			"LoginHostname = 127.0.0.1\nLoginPort = " + strconv.Itoa(gamePort) + "\n",
		"serverNames.xml":       "<?xml version='1.0' encoding='utf-8'?>\n<list>\n\t<server id=\"1\" name=\"Bartz\" />\n</list>\n",
		"logging.properties":    "",
		"banned_ips.properties": "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "log"), 0o755); err != nil {
		t.Fatal(err)
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
