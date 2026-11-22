package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/db"
	"github.com/fatal10110/acis_golang/internal/config"
)

const configBundle = "../../ops/docker/config"

// regionLine is a geoengine.properties region key, "X_Y".
var regionLine = regexp.MustCompile(`^[0-9]+_[0-9]+$`)

// bundleFiles are the config files both servers read; the bundle must hold
// every one of them.
var bundleFiles = []string{
	"banned_ips.properties", "clans.properties", "events.properties", "geoengine.properties",
	"logging.properties", "loginserver.properties", "npcs.properties", "players.properties",
	"server.properties", "siege.properties",
}

// acisInitHome lays out the directory acis-init works in, as the init
// service mounts it: a datapack data/ and, when cfg is set, a copy of that
// config directory. It returns the home directory.
func acisInitHome(t *testing.T, cfg string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "data", "xml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if cfg != "" {
		if err := os.CopyFS(filepath.Join(home, "config"), os.DirFS(cfg)); err != nil {
			t.Fatal(err)
		}
	}
	return home
}

// runAcisInit runs ops/docker/acis-init.sh in home with env added, and a
// stand-in gsregister on PATH that records its working directory and
// arguments in home/gsregister.calls. It returns the combined output.
func runAcisInit(t *testing.T, home string, env ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("acis-init.sh needs a POSIX sh")
	}
	bin := t.TempDir()
	fake := "#!/bin/sh\necho \"$PWD $*\" >>gsregister.calls\n"
	if err := os.WriteFile(filepath.Join(bin, "gsregister"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("../../ops/docker/acis-init.sh")
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := filepath.Abs(configBundle)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", script)
	cmd.Env = append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"ACIS_HOME="+home,
		"ACIS_CONFIG_BUNDLE="+bundle,
	)
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func loadProps(t *testing.T, path string) *config.Properties {
	t.Helper()
	props, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return props
}

// geoRegions returns the regions a geoengine.properties enables.
func geoRegions(t *testing.T, path string) []string {
	t.Helper()
	var regions []string
	for _, key := range loadProps(t, path).Keys() {
		if regionLine.MatchString(key) {
			regions = append(regions, key)
		}
	}
	slices.Sort(regions)
	return regions
}

// TestConfigBundleLoads checks that every file of the bundle loads with the
// servers' readers, that the servers read every key it sets, and that it
// already points at the compose services.
func TestConfigBundleLoads(t *testing.T) {
	for _, name := range bundleFiles {
		props := loadProps(t, filepath.Join(configBundle, name))
		server := config.GameServer
		switch name {
		case "loginserver.properties":
			server = config.LoginServer
		case "banned_ips.properties", "logging.properties":
			continue
		}
		if unsupported, _ := config.UnreadKeys(server, name, props); len(unsupported) > 0 {
			t.Errorf("%s: keys the server does not read: %v", name, unsupported)
		}
	}
	serverProps := loadProps(t, filepath.Join(configBundle, "server.properties"))
	hexProps, err := config.ParseString("ServerID = 1\nHexID = -7fff\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("bundle server.properties does not load: %v", err)
	}
	if cfg.LoginAddr != "loginserver:9014" || cfg.Database.URL != "jdbc:mariadb://db/acis" {
		t.Errorf("bundle server.properties: LoginAddr %q, URL %q; want the compose services", cfg.LoginAddr, cfg.Database.URL)
	}
	if got := loadProps(t, filepath.Join(configBundle, "loginserver.properties")).String("URL", ""); got != "jdbc:mariadb://db/acis" {
		t.Errorf("bundle loginserver.properties URL = %q, want the db service", got)
	}
}

// TestAcisInitFillsEmptyConfigFromBundle runs the init service's script over
// an empty config directory with every per-stack variable set, then loads
// the result with the servers' own readers.
func TestAcisInitFillsEmptyConfigFromBundle(t *testing.T) {
	home := acisInitHome(t, "")
	env := []string{
		"ACIS_DB_ROOT_PASSWORD=s3cret",
		"ACIS_GAME_PORT=7788",
		"ACIS_PUBLIC_HOST=203.0.113.7",
		"ACIS_GAMESERVER_HOSTNAME=0.0.0.0",
		"ACIS_MAX_ONLINE_USERS=40",
		"ACIS_AUTO_CREATE_ACCOUNTS=False",
		"ACIS_LOGIN_TRY_BEFORE_BAN=7",
		"ACIS_ACCEPT_FILTER=False",
		"ACIS_ACCEPT_FILTER_ALLOWLIST=127.0.0.1, 10.0.0.0/8",
		"ACIS_GEO_REGIONS=17_11,16_10",
	}
	if out, err := runAcisInit(t, home, env...); err != nil {
		t.Fatalf("acis-init.sh: %v\n%s", err, out)
	}
	cfgDir := filepath.Join(home, "config")
	for _, name := range bundleFiles {
		if _, err := os.Stat(filepath.Join(cfgDir, name)); err != nil {
			t.Errorf("bundle file %s not installed: %v", name, err)
		}
	}

	serverProps := loadProps(t, filepath.Join(cfgDir, "server.properties"))
	hexProps, err := config.ParseString("ServerID = 1\nHexID = -7fff\n")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := gameServerConfigFromProperties(gameServerPaths{}, serverProps, hexProps)
	if err != nil {
		t.Fatalf("written server.properties does not load: %v", err)
	}
	if cfg.LoginAddr != "loginserver:9014" {
		t.Errorf("LoginAddr = %q, want loginserver:9014", cfg.LoginAddr)
	}
	if cfg.ListenAddr != "0.0.0.0:7788" {
		t.Errorf("ListenAddr = %q, want 0.0.0.0:7788", cfg.ListenAddr)
	}
	if cfg.Auth.HostName != "203.0.113.7" {
		t.Errorf("advertised Hostname = %q, want 203.0.113.7", cfg.Auth.HostName)
	}
	if cfg.Auth.MaxPlayers != 40 {
		t.Errorf("MaxPlayers = %d, want 40", cfg.Auth.MaxPlayers)
	}
	want := db.Config{URL: "jdbc:mariadb://db/acis", Login: "root", Password: "s3cret"}
	if cfg.Database != want {
		t.Errorf("game Database = %+v, want %+v", cfg.Database, want)
	}
	loginProps := loadProps(t, filepath.Join(cfgDir, "loginserver.properties"))
	for key, want := range map[string]string{
		"URL":                   "jdbc:mariadb://db/acis",
		"Login":                 "root",
		"Password":              "s3cret",
		"AutoCreateAccounts":    "False",
		"LoginTryBeforeBan":     "7",
		"EnableAcceptFilter":    "False",
		"AcceptFilterAllowlist": "127.0.0.1, 10.0.0.0/8",
	} {
		if got := loginProps.String(key, "<missing>"); got != want {
			t.Errorf("loginserver.properties %s = %q, want %q", key, got, want)
		}
	}
	for key, want := range map[string]string{"EnableAcceptFilter": "False", "AcceptFilterAllowlist": "127.0.0.1, 10.0.0.0/8"} {
		if got := serverProps.String(key, "<missing>"); got != want {
			t.Errorf("server.properties %s = %q, want %q", key, got, want)
		}
	}
	for name, server := range map[string]config.Server{"server.properties": config.GameServer, "loginserver.properties": config.LoginServer} {
		if unsupported, _ := config.UnreadKeys(server, name, loadProps(t, filepath.Join(cfgDir, name))); len(unsupported) > 0 {
			t.Errorf("%s: written keys the server does not read: %v", name, unsupported)
		}
	}
	if got := geoRegions(t, filepath.Join(cfgDir, "geoengine.properties")); !slices.Equal(got, []string{"16_10", "17_11"}) {
		t.Errorf("geoengine regions = %v, want [16_10 17_11]", got)
	}
	for _, dir := range []string{"log/loginserver", "log/gameserver", "cache", "data/crests"} {
		if info, err := os.Stat(filepath.Join(home, dir)); err != nil || !info.IsDir() {
			t.Errorf("%s not created: %v", dir, err)
		}
	}

	// Every start runs it again: the files stay byte for byte, and each run
	// asks gsregister for the same registration.
	before := map[string][]byte{}
	for _, name := range bundleFiles {
		before[name], _ = os.ReadFile(filepath.Join(cfgDir, name))
	}
	if out, err := runAcisInit(t, home, env...); err != nil {
		t.Fatalf("second acis-init.sh run: %v\n%s", err, out)
	}
	for _, name := range bundleFiles {
		if after, _ := os.ReadFile(filepath.Join(cfgDir, name)); string(after) != string(before[name]) {
			t.Errorf("second run changed %s", name)
		}
	}
	calls, err := os.ReadFile(filepath.Join(home, "gsregister.calls"))
	if err != nil {
		t.Fatal(err)
	}
	call := home + " -config config/loginserver.properties -names data/serverNames.xml -register 1 -out config/hexid.txt\n"
	if string(calls) != call+call {
		t.Errorf("gsregister calls = %q, want two of %q", calls, call)
	}
}

// TestAcisInitKeepsInitConfigOutput runs the init service over a config
// init-config.sh wrote from a reference tree: that config stays, only the
// keys the compose network fixes are enforced, and no geoengine.properties
// is needed while no region list is set.
func TestAcisInitKeepsInitConfigOutput(t *testing.T) {
	dst, _, out, err := runInitConfig(t, initConfigServerFixture, initConfigLoginFixture)
	if err != nil {
		t.Fatalf("init-config.sh: %v\n%s", err, out)
	}
	home := acisInitHome(t, dst)
	if out, err := runAcisInit(t, home, "ACIS_DB_ROOT_PASSWORD=other"); err != nil {
		t.Fatalf("acis-init.sh without geoengine.properties: %v\n%s", err, out)
	}

	// A reference tree's files carry CRLF line ends.
	geo := filepath.Join(home, "config", "geoengine.properties")
	if err := os.WriteFile(geo, []byte("GeoDataPath = ./data/geodata/\r\n16_10\r\n#16_13 - not supported by L2 client\r\n17_10\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runAcisInit(t, home, "ACIS_DB_ROOT_PASSWORD=other", "ACIS_GEO_REGIONS=18_10"); err != nil {
		t.Fatalf("acis-init.sh: %v\n%s", err, out)
	}
	cfgDir := filepath.Join(home, "config")
	if data, err := os.ReadFile(filepath.Join(cfgDir, "players.properties")); err != nil || string(data) != "MaxBuffsAmount = 20\n" {
		t.Errorf("players.properties = %q, %v; want the reference file kept", data, err)
	}
	if _, err := os.Stat(filepath.Join(cfgDir, "npcs.properties")); err == nil {
		t.Error("the bundle was copied over an existing config")
	}
	serverProps := loadProps(t, filepath.Join(cfgDir, "server.properties"))
	for key, want := range map[string]string{
		"Hostname":       "203.0.113.7",
		"Password":       "other",
		"GameserverPort": "7777",
		"LoginHost":      "loginserver",
	} {
		if got := serverProps.String(key, "<missing>"); got != want {
			t.Errorf("server.properties %s = %q, want %q", key, got, want)
		}
	}
	if got := geoRegions(t, geo); !slices.Equal(got, []string{"18_10"}) {
		t.Errorf("geoengine regions = %v, want [18_10]", got)
	}
	if data, _ := os.ReadFile(geo); !strings.Contains(string(data), "#16_13 - not supported by L2 client") {
		t.Errorf("region rewrite dropped a comment line:\n%s", data)
	}
}

func TestAcisInitRejectsBadSettings(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		env        []string
	}{
		{"no password", "ACIS_DB_ROOT_PASSWORD", nil},
		{"bad region", "abc is not a region", []string{"ACIS_DB_ROOT_PASSWORD=x", "ACIS_GEO_REGIONS=16_10 abc"}},
		{"backslash", "backslash", []string{"ACIS_DB_ROOT_PASSWORD=x", `ACIS_PUBLIC_HOST=a\b`}},
		{"newline", "newline", []string{"ACIS_DB_ROOT_PASSWORD=x", "ACIS_PUBLIC_HOST=a\nURL = x"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := acisInitHome(t, "")
			env := append([]string{"ACIS_DB_ROOT_PASSWORD="}, tc.env...)
			out, err := runAcisInit(t, home, env...)
			if err == nil || !strings.Contains(out, tc.want) {
				t.Fatalf("acis-init.sh: err %v, output %q; want a failure naming %q", err, out, tc.want)
			}
			if _, err := os.Stat(filepath.Join(home, "gsregister.calls")); err == nil {
				t.Error("registered despite the bad setting")
			}
		})
	}
}

// composeServices splits docker-compose.yml into its services' blocks: the
// lines below each two-space "name:" line, in the file's block-YAML shape.
func composeServices(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	blocks := map[string]string{}
	var service string
	for line := range strings.Lines(string(data)) {
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
		case service != "":
			blocks[service] += trimmed + "\n"
		}
	}
	return blocks
}

// TestComposeStacksDoNotCollide checks that two stacks can share a host:
// no fixed container name, and the published client ports come from the
// environment, with the game port the same on both sides because the login
// server hands clients the port the game server listens on.
func TestComposeStacksDoNotCollide(t *testing.T) {
	services := composeServices(t)
	for name, block := range services {
		if strings.Contains(block, "container_name:") {
			t.Errorf("service %s has a fixed container_name", name)
		}
	}
	if !strings.Contains(services["loginserver"], `- "${ACIS_LOGIN_PORT:-2106}:2106"`) {
		t.Errorf("loginserver ports do not come from ACIS_LOGIN_PORT:\n%s", services["loginserver"])
	}
	if !strings.Contains(services["gameserver"], `- "${ACIS_GAME_PORT:-7777}:${ACIS_GAME_PORT:-7777}"`) {
		t.Errorf("gameserver ports do not come from ACIS_GAME_PORT on both sides:\n%s", services["gameserver"])
	}
	if !strings.Contains(services["init"], "ACIS_GAME_PORT: ${ACIS_GAME_PORT:-7777}") {
		t.Errorf("init service does not get ACIS_GAME_PORT, so GameserverPort can drift from the published port:\n%s", services["init"])
	}
}

// TestComposeServersWaitForInit checks that both servers start only after
// the init service finished: the login server reads the registered game
// servers once, at startup.
func TestComposeServersWaitForInit(t *testing.T) {
	services := composeServices(t)
	if !strings.Contains(services["init"], `command: ["acis-init"]`) || !strings.Contains(services["init"], `restart: "no"`) {
		t.Fatalf("init service does not run acis-init once:\n%s", services["init"])
	}
	for _, name := range []string{"loginserver", "gameserver"} {
		if !strings.Contains(services[name], "init:\ncondition: service_completed_successfully\n") {
			t.Errorf("%s does not wait for init to complete:\n%s", name, services[name])
		}
	}
}
