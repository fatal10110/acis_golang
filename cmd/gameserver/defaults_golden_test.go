package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// referenceDefault is one key's default as the reference configuration
// loader spells it, read from internal/config/testdata/reference_defaults.golden.
type referenceDefault struct {
	file, key string
	// kind is bool, int, long, double, string, strings, ints or none (a key
	// the reference reads with no default).
	kind string
	// value is the literal as written, with a list's elements joined by ",".
	value string
}

func (d referenceDefault) line() string {
	return d.file + " " + d.key + " " + d.kind + " " + strconv.Quote(d.value)
}

// goldenDefaults reads testdata/reference_defaults.golden.
func goldenDefaults(t *testing.T) []referenceDefault {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "config", "testdata", "reference_defaults.golden"))
	if err != nil {
		t.Fatal(err)
	}
	var out []referenceDefault
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 4)
		if len(parts) != 4 {
			t.Fatalf("golden line %q is not \"<file> <key> <kind> <default>\"", line)
		}
		value, err := strconv.Unquote(parts[3])
		if err != nil {
			t.Fatalf("golden line %q: %v", line, err)
		}
		out = append(out, referenceDefault{file: parts[0], key: parts[1], kind: parts[2], value: value})
	}
	return out
}

// goDefault is one reader call of this server's code: the key it reads and
// the default it falls back on.
type goDefault struct {
	pos    string
	method string
	// value is the default as a constant, or the elements of a list default.
	value  constant.Value
	elems  []constant.Value
	isList bool
}

// readerMethods are the config.Fields and config.Properties getters that take
// a key and its default.
var readerMethods = map[string]bool{
	"String": true, "Bool": true, "Int": true, "Int64": true, "Float64": true,
	"Strings": true, "Bools": true, "Ints": true, "Int64s": true, "Float64s": true,
	"IntPairs": true, "IntPairsComma": true,
}

// goDefaults scans the non-test Go files under roots for reader calls whose
// key is one of keys, and returns each key's call sites. The packages are
// type-checked from source, bodies skipped, so a default spelled as a
// constant of another package of the module resolves; anything else
// (a variable, a call) stays unresolved, a nil value.
func goDefaults(t *testing.T, keys map[string]bool, roots ...string) map[string][]goDefault {
	t.Helper()
	ld := newSourceLoader()
	var dirs []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() && !slices.Contains(dirs, path) {
				dirs = append(dirs, path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	out := map[string][]goDefault{}
	for _, dir := range dirs {
		if !importsConfig(t, dir) {
			continue
		}
		pkg := ld.load(t, dir, true)
		if pkg == nil {
			continue
		}
		for _, f := range pkg.files {
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || len(call.Args) != 2 {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !readerMethods[sel.Sel.Name] {
					return true
				}
				k := pkg.info.Types[call.Args[0]].Value
				if k == nil || k.Kind() != constant.String {
					return true
				}
				key := constant.StringVal(k)
				if !keys[key] {
					return true
				}
				d := goDefault{pos: ld.fset.Position(call.Pos()).String(), method: sel.Sel.Name}
				switch arg := call.Args[1].(type) {
				case *ast.CompositeLit:
					d.isList = true
					for _, e := range arg.Elts {
						d.elems = append(d.elems, pkg.info.Types[e].Value)
					}
				default:
					d.value = pkg.info.Types[arg].Value
					if id, ok := arg.(*ast.Ident); ok && id.Name == "nil" {
						d.isList = true
					}
				}
				out[key] = append(out[key], d)
				return true
			})
		}
	}
	return out
}

// importsConfig reports whether a non-test file of dir imports the config
// package, the only way to reach a reader.
func importsConfig(t *testing.T, dir string) bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(src, []byte(`"`+modulePath+`/internal/config"`)) {
			return true
		}
	}
	return false
}

const modulePath = "github.com/fatal10110/acis_golang"

// sourcePackage is one package of the module, parsed and type-checked.
type sourcePackage struct {
	files []*ast.File
	info  *types.Info
	types *types.Package
}

// sourceLoader type-checks the module's packages from source, function
// bodies skipped, standard-library imports from the standard library's
// source and every other import as an empty package.
type sourceLoader struct {
	fset  *token.FileSet
	std   types.Importer
	cache map[string]*sourcePackage
}

func newSourceLoader() *sourceLoader {
	fset := token.NewFileSet()
	return &sourceLoader{fset: fset, std: importer.ForCompiler(fset, "source", nil), cache: map[string]*sourcePackage{}}
}

// load returns the non-test package in dir, nil when it has no Go file.
// Function bodies are checked only when bodies is set, which a scan needs
// for the types of the expressions in them and an import does not.
func (l *sourceLoader) load(t *testing.T, dir string, bodies bool) *sourcePackage {
	t.Helper()
	dir = filepath.Clean(dir)
	cacheKey := fmt.Sprintf("%s %t", dir, bodies)
	if pkg, ok := l.cache[cacheKey]; ok {
		return pkg
	}
	l.cache[cacheKey] = nil // an import cycle resolves to nothing
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(l.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		return nil
	}
	pkg := &sourcePackage{files: files, info: &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}}
	conf := types.Config{
		Importer:         moduleImporter{l: l, t: t},
		Error:            func(error) {},
		IgnoreFuncBodies: !bodies,
	}
	pkg.types, _ = conf.Check(dir, l.fset, files, pkg.info)
	l.cache[cacheKey] = pkg
	return pkg
}

// moduleImporter resolves an import for the package being checked.
type moduleImporter struct {
	l *sourceLoader
	t *testing.T
}

func (m moduleImporter) Import(path string) (*types.Package, error) {
	if rel, ok := strings.CutPrefix(path, modulePath+"/"); ok {
		if pkg := m.l.load(m.t, filepath.Join("..", "..", filepath.FromSlash(rel)), false); pkg != nil {
			return pkg.types, nil
		}
	} else if !strings.Contains(strings.SplitN(path, "/", 2)[0], ".") {
		if pkg, err := m.l.std.Import(path); err == nil {
			return pkg, nil
		}
	}
	pkg := types.NewPackage(path, path[strings.LastIndex(path, "/")+1:])
	pkg.MarkComplete()
	return pkg, nil
}

// matches reports whether the Go default d is the reference default ref.
func (d goDefault) matches(ref referenceDefault) (ok, checked bool) {
	if d.isList {
		if ref.kind != "strings" && ref.kind != "ints" && ref.kind != "none" {
			return false, true
		}
		return d.listMatches(ref), true
	}
	if d.value == nil {
		return false, false
	}
	switch ref.kind {
	case "bool":
		return d.value.Kind() == constant.Bool && d.value.String() == ref.value, true
	case "int", "long", "double":
		want := constant.MakeFromLiteral(ref.value, token.FLOAT, 0)
		if ref.kind != "double" {
			want = constant.MakeFromLiteral(ref.value, token.INT, 0)
		}
		if d.value.Kind() != constant.Int && d.value.Kind() != constant.Float {
			return false, true
		}
		// A typed float constant is rounded to float64, the reference's
		// double; compare as the program reads them.
		got, _ := constant.Float64Val(d.value)
		wantF, _ := constant.Float64Val(want)
		return got == wantF, true
	case "string":
		return d.value.Kind() == constant.String && constant.StringVal(d.value) == ref.value, true
	case "none":
		// The reference reads no default; an empty one is the same as none.
		return d.value.Kind() == constant.String && constant.StringVal(d.value) == "", false
	}
	return false, true
}

func (d goDefault) listMatches(ref referenceDefault) bool {
	if ref.kind == "none" {
		return len(d.elems) == 0
	}
	var want []string
	if ref.value != "" {
		want = strings.Split(ref.value, ",")
	}
	if len(want) != len(d.elems) {
		return false
	}
	for i, e := range d.elems {
		if e == nil || strings.Trim(e.ExactString(), `"`) != want[i] {
			return false
		}
	}
	return true
}

// equivalentDefaults are the keys whose reader spells a default other than
// the reference's and means the same.
var equivalentDefaults = map[string]string{
	"GeoDataPath": "an empty path resolves to <data root>/data/geodata (geodata.ResolveDir), the reference's ./data/geodata/",
}

// pathsLoaders are the game server's loaders of a properties file, each
// taking the file paths (and nothing the empty files cannot give).
var pathsLoaders = []any{
	loadBoardConfig,
	loadClassMasterConfig,
	loadPlayerSpawnProtection,
	loadRespawnRestoreHP,
	loadDeathPenaltyChance,
	loadMaxBuffsAmount,
	loadPerfectShieldBlockRate,
	loadMagicFailures,
	loadCancelLesserEffect,
	loadStoreSkillCooltime,
	loadSkillEnchantSPBookNeeded,
	loadWeightLimitMultiplier,
	loadRegenMultipliers,
	loadEnchantConfig,
	loadAugmentationChances,
	loadInventorySlots,
	loadStorageSlots,
	loadAutoLearnSkills,
	loadKarmaPlayerCanTeleport,
	loadKarmaServiceGates,
	loadKarmaPlayerCanTrade,
	loadAllowDelevel,
	loadRateKarmaExpLost,
	loadMerchantConfig,
	loadFreightConfig,
	loadCharacterSelectDelay,
	loadServerBypassDelay,
	loadManufactureDelay,
	loadCraftingEnabled,
	loadMultisellDelay,
	loadManorDelay,
	loadRollDiceDelay,
	loadSocialDelay,
	loadSubclassDelay,
	loadSubclassWithoutQuests,
	loadBlacksmithUseRecipes,
	loadAllowDiscardItem,
	loadLogItems,
	loadSpawnMultiplier,
	loadSpawnEvents,
	loadRandomWalkRate,
	loadMaxGeoPathFailCount,
	loadDisableRaidCurse,
	loadFreeTeleport,
	loadShowNPCCrest,
	loadShowNPCLevel,
	loadRaidMultipliers,
	loadNpcAIConfig,
	loadMaxDriftRange,
	loadBossWaitTimes,
	loadAdminConfig,
	loadChatConfig,
	loadDeathDropRules,
	loadDropItemDelay,
	loadWarehouseAllowed,
	loadNoSpawns,
	loadFakeDeathUpProtection,
	loadShowSummonCrest,
	loadPetitionConfig,
	loadEventRules,
	loadFestivalConfig,
	loadFishingChampionshipConfig,
	loadHeroMinMatches,
	loadLotteryConfig,
	loadManorSettings,
	loadOlympiadConfig,
	loadSchemeBufferConfig,
	loadSiegeConfig,
	loadHallSiegeConfig,
	loadSignsPriestConfig,
	loadWeddingConfig,
	loadWyvernRules,
	loadGameplayConfig,
	loadPetConfig,
	loadPvPFlagOptions,
	loadClanConfig,
	loadCastleManorConfig,
	loadGameServerProperties,
}

// runtimeDefaults runs every loader of the game server on empty properties
// files and returns, by key, the defaults each one logged for a key it
// found missing. These are the defaults a call site passes as a value of a
// defaults struct, which a scan of the source cannot resolve.
func runtimeDefaults(t *testing.T) map[string][]any {
	t.Helper()
	dir := t.TempDir()
	file := func(name string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	paths := gameServerPaths{
		ConfigPath:        file("server.properties"),
		PlayersConfigPath: file("players.properties"),
		GeoConfigPath:     file("geoengine.properties"),
		NpcsConfigPath:    file("npcs.properties"),
		ClansConfigPath:   file("clans.properties"),
		EventsConfigPath:  file("events.properties"),
		SiegeConfigPath:   file("siege.properties"),
		DataRoot:          dir,
	}
	var logged bytes.Buffer
	config.SetLogger(zerolog.New(&logged))
	t.Cleanup(func() { config.SetLogger(zerolog.Nop()) })

	args := []reflect.Value{reflect.ValueOf(paths)}
	for _, load := range pathsLoaders {
		fn := reflect.ValueOf(load)
		in := args
		if fn.Type().NumIn() == 2 {
			in = append(slices.Clone(args), reflect.Zero(fn.Type().In(1)))
		}
		out := fn.Call(in)
		if err, _ := out[len(out)-1].Interface().(error); err != nil {
			t.Errorf("%s on empty properties: %v", runtime.FuncForPC(fn.Pointer()).Name(), err)
		}
	}
	props, err := config.ParseString("")
	if err != nil {
		t.Fatal(err)
	}
	for name, load := range map[string]func() error{
		"engine":      func() error { _, err := engine.OptionsFromProperties(props); return err },
		"pathfind":    func() error { _, err := pathfind.OptionsFromProperties(props); return err },
		"pvpflag":     func() error { _, err := task.PvPFlagOptionsFromProperties(props); return err },
		"questrates":  func() error { _, err := loadQuestRates(props); return err },
		"grounditems": func() error { _, err := task.GroundItemOptionsFromProperties(props); return err },
	} {
		if err := load(); err != nil {
			t.Errorf("%s options on empty properties: %v", name, err)
		}
	}

	out := map[string][]any{}
	for _, line := range bytes.Split(logged.Bytes(), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Key     string `json:"key"`
			Default any    `json:"default"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("logged line %q: %v", line, err)
		}
		out[entry.Key] = append(out[entry.Key], entry.Default)
	}
	return out
}

// matchesRuntime reports whether a logged default is the reference default.
func matchesRuntime(ref referenceDefault, v any) (ok, checked bool) {
	switch ref.kind {
	case "bool":
		b, isBool := v.(bool)
		return isBool && strconv.FormatBool(b) == ref.value, true
	case "int", "long", "double":
		f, isNum := v.(float64)
		want, err := strconv.ParseFloat(strings.TrimSuffix(ref.value, "."), 64)
		return isNum && err == nil && math.Abs(f-want) < 1e-12, true
	case "string":
		s, isStr := v.(string)
		return isStr && s == ref.value, true
	case "strings", "ints":
		list, isList := v.([]any)
		if !isList {
			return false, true
		}
		var got []string
		for _, e := range list {
			switch e := e.(type) {
			case float64:
				got = append(got, strconv.FormatFloat(e, 'f', -1, 64))
			case string:
				got = append(got, e)
			}
		}
		return strings.Join(got, ",") == ref.value, true
	}
	return false, false
}

// unread reports whether this server reads no such key, so no default of
// its own is to be checked: the key tables list it as ignored.
func unread(t *testing.T, d referenceDefault) bool {
	t.Helper()
	props, err := config.ParseString(d.key + " = x")
	if err != nil {
		t.Fatal(err)
	}
	server := config.GameServer
	if d.file == "loginserver.properties" {
		server = config.LoginServer
	}
	unsupported, ignored := config.UnreadKeys(server, d.file, props)
	return len(unsupported)+len(ignored) > 0
}

// TestReaderDefaultsMatchTheReferenceGolden holds every default the game
// server's readers fall back on to the reference loader's: each key the
// golden lists is read with the default the golden records for it, whether
// the call site spells it (a scan of the source) or takes it from a defaults
// struct (the loaders run on empty files), and no key goes unchecked. It does
// not run in parallel: the loaders log through one process-wide logger.
func TestReaderDefaultsMatchTheReferenceGolden(t *testing.T) {
	golden := goldenDefaults(t)
	byKey := map[string][]referenceDefault{}
	keys := map[string]bool{}
	for _, d := range golden {
		byKey[d.key] = append(byKey[d.key], d)
		keys[d.key] = true
	}
	sites := goDefaults(t, keys, filepath.Join("..", "..", "cmd"), filepath.Join("..", "..", "internal"))
	runtime := runtimeDefaults(t)

	var drift []string
	covered := map[string]bool{}
	for key, refs := range byKey {
		if _, ok := equivalentDefaults[key]; ok {
			covered[key] = true
			continue
		}
		for _, site := range sites[key] {
			any, checked := false, false
			for _, ref := range refs {
				ok, c := site.matches(ref)
				any, checked = any || ok, checked || c
			}
			switch {
			case checked && !any:
				drift = append(drift, fmt.Sprintf("%s: %s(%q, %v), the reference reads %s", site.pos, site.method, key, site.value, refs[0].line()))
			case checked:
				covered[key] = true
			}
		}
		for _, v := range runtime[key] {
			any, checked := false, false
			for _, ref := range refs {
				ok, c := matchesRuntime(ref, v)
				any, checked = any || ok, checked || c
			}
			switch {
			case checked && !any:
				drift = append(drift, fmt.Sprintf("loaders default %q to %v, the reference reads %s", key, v, refs[0].line()))
			case checked:
				covered[key] = true
			}
		}
	}
	var unchecked []string
	for _, d := range golden {
		if d.kind != "none" && !covered[d.key] && !unread(t, d) {
			unchecked = append(unchecked, d.line())
		}
	}
	slices.Sort(drift)
	if len(drift) > 0 {
		t.Errorf("reader defaults differ from the reference:\n%s", strings.Join(drift, "\n"))
	}
	if len(unchecked) > 0 {
		t.Errorf("%d reference defaults no reader is checked against:\n%s", len(unchecked), strings.Join(unchecked, "\n"))
	}
}
