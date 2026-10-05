package main

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
)

// TestComposeStopGraceCoversGameServerStop checks that docker-compose.yml
// gives the gameserver container longer than the process's own stop budget.
// Docker's default grace is 10s, after which it SIGKILLs a shutdown that is
// still saving players and world state.
func TestComposeStopGraceCoversGameServerStop(t *testing.T) {
	const compose = "../../docker-compose.yml"
	f, err := os.Open(compose)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// The file is plain block YAML: services at two spaces, their keys at
	// four. Track which service block a line belongs to.
	var service, grace string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case indent == 0:
			service = ""
		case indent == 2 && strings.HasSuffix(trimmed, ":"):
			service = strings.TrimSuffix(trimmed, ":")
		case indent == 4 && service == "gameserver":
			if v, ok := strings.CutPrefix(trimmed, "stop_grace_period:"); ok {
				grace = strings.TrimSpace(v)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if grace == "" {
		t.Fatalf("%s: gameserver service has no stop_grace_period, so Docker kills it after 10s", compose)
	}
	stop, err := time.ParseDuration(grace)
	if err != nil {
		t.Fatalf("%s: gameserver stop_grace_period %q: %v", compose, grace, err)
	}
	if stop <= gameServerStopTimeout {
		t.Fatalf("%s: gameserver stop_grace_period = %s, want above gameServerStopTimeout = %s", compose, stop, gameServerStopTimeout)
	}
}

// Shipped reference lines of the keys ops/docker/init-config.sh rewrites,
// with the commented-out alternative URLs the reference files carry. Login
// is a dedicated user, as in a migrated Java setup: the compose db creates
// only root, so the script must rewrite it.
const (
	initConfigServerFixture = `# This is transmitted to the clients
Hostname = *
GameserverHostname = *
GameserverPort = 7777
LoginHost = 127.0.0.1
LoginPort = 9014
URL = jdbc:mariadb://localhost/acis
#URL = jdbc:mysql://localhost/acis?serverTimezone=UTC
Login = l2j
Password =
ServerListBrackets = False`
	initConfigLoginFixture = `Hostname = localhost
LoginserverHostname = *
LoginHostname = *
LoginPort = 9014
URL = jdbc:mariadb://localhost/acis
#URL = jdbc:mysql://localhost/acis?serverTimezone=UTC
Login = l2j
Password =
AutoCreateAccounts = True`
)

// runInitConfig runs ops/docker/init-config.sh on a reference config
// directory holding server and login, with every path it writes inside the
// test's temp directory. It returns the written config directory, the
// datapack root, the script's combined output, and its error.
func runInitConfig(t *testing.T, server, login string) (dst, datapack, out string, err error) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("init-config.sh needs a POSIX sh")
	}
	root := t.TempDir()
	src := filepath.Join(root, "reference-config")
	dst = filepath.Join(root, "config")
	datapack = filepath.Join(root, "aCis_datapack")
	for path, content := range map[string]string{
		filepath.Join(src, "server.properties"):      server,
		filepath.Join(src, "loginserver.properties"): login,
		filepath.Join(src, "players.properties"):     "MaxBuffsAmount = 20\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(datapack, "data", "xml"), 0o755); err != nil {
		t.Fatal(err)
	}

	script, err := filepath.Abs("../../ops/docker/init-config.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, src)
	cmd.Env = append(os.Environ(),
		"ACIS_CONFIG_DIR="+dst,
		"ACIS_DATAPACK_DIR="+datapack,
		"ACIS_LOG_DIR="+filepath.Join(root, "log"),
		"ACIS_DB_ROOT_PASSWORD=s3cret",
		"ACIS_PUBLIC_HOST=203.0.113.7",
	)
	output, err := cmd.CombinedOutput()
	return dst, datapack, string(output), err
}

// TestInitConfigPointsServersAtComposeServices runs the shipped reference
// key lines through ops/docker/init-config.sh and loads the result with the
// servers' own config readers: the game server must reach the db and
// loginserver services as the db's root user and advertise the public
// host, and every other key and file must stay as shipped.
func TestInitConfigPointsServersAtComposeServices(t *testing.T) {
	dst, datapack, out, err := runInitConfig(t, initConfigServerFixture, initConfigLoginFixture)
	if err != nil {
		t.Fatalf("init-config.sh: %v\n%s", err, out)
	}

	serverProps, err := config.LoadFile(filepath.Join(dst, "server.properties"))
	if err != nil {
		t.Fatal(err)
	}
	hexProps, err := config.ParseString("ServerID = 1\nHexID = -7fff\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("rewritten server.properties does not load: %v", err)
	}
	if cfg.LoginAddr != "loginserver:9014" {
		t.Errorf("LoginAddr = %q, want loginserver:9014", cfg.LoginAddr)
	}
	if cfg.Auth.HostName != "203.0.113.7" {
		t.Errorf("advertised Hostname = %q, want 203.0.113.7", cfg.Auth.HostName)
	}
	if cfg.ListenAddr != ":7777" {
		t.Errorf("ListenAddr = %q, want :7777 (GameserverHostname kept)", cfg.ListenAddr)
	}
	want := db.Config{URL: "jdbc:mariadb://db/acis", Login: "root", Password: "s3cret"}
	if cfg.Database != want {
		t.Errorf("game Database = %+v, want %+v", cfg.Database, want)
	}
	pool, err := db.Open(cfg.Database)
	if err != nil {
		t.Fatalf("rewritten URL is not a usable data source: %v", err)
	}
	pool.Close()
	if v := serverProps.String("ServerListBrackets", ""); v != "False" {
		t.Errorf("ServerListBrackets = %q, want the shipped False", v)
	}

	loginProps, err := config.LoadFile(filepath.Join(dst, "loginserver.properties"))
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"URL":                 "jdbc:mariadb://db/acis",
		"Login":               "root",
		"Password":            "s3cret",
		"Hostname":            "localhost",
		"LoginserverHostname": "*",
		"LoginHostname":       "*",
		"AutoCreateAccounts":  "True",
	} {
		if got := loginProps.String(key, "<missing>"); got != want {
			t.Errorf("loginserver.properties %s = %q, want %q", key, got, want)
		}
	}

	for _, name := range []string{"server.properties", "loginserver.properties"} {
		data, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "#URL = jdbc:mysql://localhost/acis?serverTimezone=UTC") {
			t.Errorf("%s: commented-out URL line was rewritten or dropped", name)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dst, "players.properties")); err != nil || string(data) != "MaxBuffsAmount = 20\n" {
		t.Errorf("players.properties = %q, %v; want the reference file copied unchanged", data, err)
	}
	if info, err := os.Stat(filepath.Join(datapack, "data", "crests")); err != nil || !info.IsDir() {
		t.Errorf("datapack data/crests not created: %v", err)
	}
}

// TestInitConfigRefusesExistingConfig keeps a second run from overwriting
// an operator's edited config.
func TestInitConfigRefusesExistingConfig(t *testing.T) {
	dst, _, out, err := runInitConfig(t, initConfigServerFixture, initConfigLoginFixture)
	if err != nil {
		t.Fatalf("first init-config.sh run: %v\n%s", err, out)
	}
	edited := filepath.Join(dst, "server.properties")
	if err := os.WriteFile(edited, []byte("Hostname = edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	script, err := filepath.Abs("../../ops/docker/init-config.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script, filepath.Join(filepath.Dir(dst), "reference-config"))
	cmd.Env = append(os.Environ(),
		"ACIS_CONFIG_DIR="+dst,
		"ACIS_DATAPACK_DIR="+filepath.Join(filepath.Dir(dst), "aCis_datapack"),
		"ACIS_LOG_DIR="+filepath.Join(filepath.Dir(dst), "log"),
	)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("second init-config.sh run succeeded, want refusal\n%s", output)
	}
	if data, _ := os.ReadFile(edited); string(data) != "Hostname = edited\n" {
		t.Errorf("server.properties = %q, want the operator's edit kept", data)
	}
}

// TestInitConfigFailsOnMissingKey reports a reference file that lacks a key
// the compose network needs rewritten, rather than leaving it pointed at
// localhost.
func TestInitConfigFailsOnMissingKey(t *testing.T) {
	server := strings.Replace(initConfigServerFixture, "LoginHost = 127.0.0.1\n", "", 1)
	_, _, out, err := runInitConfig(t, server, initConfigLoginFixture)
	if err == nil {
		t.Fatalf("init-config.sh accepted a server.properties without LoginHost\n%s", out)
	}
	if !strings.Contains(out, "no LoginHost key") {
		t.Errorf("output = %q, want it to name the missing LoginHost key", out)
	}
}
