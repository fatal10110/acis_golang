package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/idfactory"
	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
)

// TestBootStagesAreOrderedByTheirConstructorInputs pins the boot order the
// client sees (used-id scan and cleanup, crest cache, clans, castles, clan
// halls, castle manor). The graph resolves constructors lazily, so the order
// holds only through each stage taking the earlier stage's product as an
// input; dropping one of those parameters would still resolve and boot, with
// the stage free to run before the data it reads is ready.
func TestBootStagesAreOrderedByTheirConstructorInputs(t *testing.T) {
	needs := []struct {
		stage, after string
		fn           any
		input        reflect.Type
	}{
		{"clans", "id allocator (used-id scan and orphan cleanup)", provideClans, reflect.TypeFor[*idfactory.Allocator]()},
		{"clans", "crest cache", provideClans, reflect.TypeFor[*datacache.Crests]()},
		{"castles", "clans", provideCastles, reflect.TypeFor[*clan.Service]()},
		{"clan hall functions", "clans", provideClanHallFunctions, reflect.TypeFor[*clan.Service]()},
		{"clan halls", "clan hall functions", provideClanHalls, reflect.TypeFor[*clanhall.Functions]()},
		{"clan halls", "clans", provideClanHalls, reflect.TypeFor[*clan.Service]()},
		{"castle manor", "castles", provideCastleManor, reflect.TypeFor[*castle.Manager]()},
		{"castle manor", "clans", provideCastleManor, reflect.TypeFor[*clan.Service]()},
	}
	for _, n := range needs {
		ft := reflect.TypeOf(n.fn)
		found := false
		for i := range ft.NumIn() {
			if ft.In(i) == n.input {
				found = true
			}
		}
		if !found {
			t.Errorf("constructor of %s does not take %s (%v): nothing orders it after that stage", n.stage, n.after, n.input)
		}
	}
}

// TestClientListenerAndLoginLinkOpenAfterEveryRestore pins that the game
// client listener and the login-link registration are opened by the last
// OnStart hook. fx runs the hooks in the order their Invoke functions append
// them, so startGameServer must be the final function of the final fx.Invoke,
// and no other function may bind a listener or start the login link.
func TestClientListenerAndLoginLinkOpenAfterEveryRestore(t *testing.T) {
	fset := token.NewFileSet()
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}

	var lastInvoke []ast.Expr
	opensListener := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				x, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				switch {
				case x.Name == "net" && sel.Sel.Name == "Listen", x.Name == "network" && (sel.Sel.Name == "Maintain" || sel.Sel.Name == "Serve"):
					opensListener[fn.Name.Name] = true
				case fn.Name.Name == "newGameServerAppOptions" && x.Name == "fx" && sel.Sel.Name == "Invoke":
					lastInvoke = call.Args
				}
				return true
			})
		}
	}

	if len(lastInvoke) == 0 {
		t.Fatal("no fx.Invoke found in newGameServerAppOptions")
	}
	last, ok := lastInvoke[len(lastInvoke)-1].(*ast.Ident)
	if !ok || last.Name != "startGameServer" {
		t.Errorf("last function of the final fx.Invoke = %v, want startGameServer", lastInvoke[len(lastInvoke)-1])
	}
	for name := range opensListener {
		if name != "startGameServer" {
			t.Errorf("%s binds the client listener or starts the login link; only startGameServer, the last OnStart hook, may", name)
		}
	}
	if !opensListener["startGameServer"] {
		t.Error("startGameServer no longer opens the client listener or the login link; update this test to the new owner")
	}
}
