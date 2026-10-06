package scenario

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Ext is the scenario file extension RunDir reads.
const Ext = ".scenario"

// killWait is how long a kill step lets pass after each lethal hit, so the
// dead NPC's dying hooks (three seconds after the death) have run: the
// reference probe's seven 500 ms ticks.
const killWait = 3500 * time.Millisecond

// Config is what a suite's generic test hands the runner.
type Config struct {
	// Catalog maps the scripts.xml paths the scenarios list to their
	// constructors.
	Catalog script.Catalog
}

// RunDir runs every scenario file of dir as a parallel subtest named after
// the file. A dir with no scenario file fails.
func RunDir(t *testing.T, dir string, cfg Config) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*"+Ext))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no %s file in %s", Ext, dir)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sc, err := Parse(filepath.Base(file), data)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(strings.TrimSuffix(sc.Name, Ext), func(t *testing.T) {
			t.Parallel()
			Run(t, sc, cfg)
		})
	}
}

// run is one scenario's booted server and the state the steps share.
type run struct {
	sc     *Scenario
	srv    *gameservertest.Server
	player int32
	// origin is where the character entered the world; NPCs stand at
	// their offsets from it.
	origin location.Location
	// pages are the shipped pages as read, texts as the server shows them.
	pages, texts map[string]string
	// roles names object ids in rendered packets; objs and npcs are each
	// role's current object id and NPC.
	roles map[int32]string
	objs  map[string]int32
	npcs  map[string]NPC
	live  map[string]*npc.Hostile
}

// Run boots the scenario's server, enters the world and runs every step,
// failing at the first expectation that does not hold.
func Run(t *testing.T, sc *Scenario, cfg Config) {
	t.Helper()
	r := &run{sc: sc, pages: loadPages(t, sc.Pages), roles: map[int32]string{}, objs: map[string]int32{}, npcs: map[string]NPC{}, live: map[string]*npc.Hostile{}}
	r.texts = pageTexts(t, r.pages)
	if sc.Trace != "" {
		data, err := os.ReadFile(filepath.Join(moduleRoot(t), filepath.FromSlash(sc.Trace)))
		if err != nil {
			t.Fatal(err)
		}
		if err := checkTrace(sc, data, r.texts); err != nil {
			t.Fatalf("%s does not replay %s: %v", sc.Name, sc.Trace, err)
		}
	}
	r.boot(t, cfg)
	for i, st := range sc.Steps {
		r.step(t, i, st)
	}
}

func (r *run) boot(t *testing.T, cfg Config) {
	t.Helper()
	list := make([]script.Listing, 0, len(r.sc.Scripts))
	catalog := script.Catalog{}
	for _, l := range r.sc.Scripts {
		ctor, ok := cfg.Catalog[l.Path]
		switch {
		case l.Stub && ok:
			t.Fatalf("%s: %s is ported; list it as a script, not a stub", r.sc.Name, l.Path)
		case l.Stub:
			stub, err := manifestStub(filepath.Join(moduleRoot(t), filepath.FromSlash(manifestPath)), l.Path)
			if err != nil {
				t.Fatalf("%s: %v", r.sc.Name, err)
			}
			ctor = func() script.Script { return stub }
		case !ok:
			t.Fatalf("%s: script %s is not in the suite's catalog", r.sc.Name, l.Path)
		}
		list = append(list, script.Listing{Path: l.Path})
		catalog[l.Path] = ctor
	}
	kinds := map[int32]script.NPCKind{}
	for _, n := range r.sc.NPCs {
		inst := &npc.Instance{Template: template(n)}
		switch {
		case npc.FolkKind(inst):
			kinds[n.ID] = script.KindFolk
		case npc.Attackable(inst):
			kinds[n.ID] = script.KindHostile
		default:
			t.Fatalf("%s: NPC %s type %s is neither a civilian nor an attackable", r.sc.Name, n.Role, n.Type)
		}
		r.npcs[n.Role] = n
	}
	r.srv = gameservertest.Boot(t,
		gameservertest.WithCharacter(r.sc.Character, r.sc.Level, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(r.pages),
		gameservertest.WithNPCScripts(kinds, list, catalog),
		gameservertest.WithItemTemplates(itemTemplates(t, r.sc.Items)),
	)
	r.player = r.srv.SoleObjectID(t)
	r.roles[r.player] = "player"
	r.enterWorld(t)
	x, y, z := r.srv.PlayerPosition(t, r.player)
	r.origin = location.Location{X: x, Y: y, Z: z}
	for _, n := range r.sc.NPCs {
		r.spawn(t, n)
	}
	r.srv.ReadQueued(t, r.srv.Client)
	r.srv.FlushPersistence(t)
	r.srv.TakeJournalWrites()
}

// spawn places a fresh NPC of n's template at n's offset under n's role.
func (r *run) spawn(t *testing.T, n NPC) {
	t.Helper()
	at := location.Location{X: r.origin.X + n.DX, Y: r.origin.Y + n.DY, Z: r.origin.Z + n.DZ}
	tmpl := template(n)
	var obj int32
	if npc.FolkKind(&npc.Instance{Template: tmpl}) {
		obj = r.srv.SpawnFolkNPCAt(t, tmpl, at).ObjectID()
	} else {
		h := r.srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		r.live[n.Role], obj = h, h.ObjectID()
	}
	r.roles[obj], r.objs[n.Role] = n.Role, obj
}

// template is the fixture template of n: a civilian one, undying like the
// shipped service NPCs, or a level 1 monster of 1000 HP.
func template(n NPC) *npc.Template {
	tmpl := gameservertest.FolkTemplate(n.Type, int(n.ID))
	if npc.FolkKind(&npc.Instance{Template: tmpl}) {
		return tmpl
	}
	return &npc.Template{
		ID: int(n.ID), TemplateID: int(n.ID), Type: n.Type, Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}
}

func (r *run) step(t *testing.T, i int, st Step) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf("%s step %d (line %d) %q: %s", r.sc.Name, i+1, st.Line, st, fmt.Sprintf(format, args...))
	}
	c := r.srv.Client
	switch st.Verb {
	case "action":
		x, y, z := r.srv.PlayerPosition(t, r.player)
		w := wire.NewPacketWriter(clientpackets.OpcodeAction)
		w.WriteInt32(r.objs[st.Args[0]])
		w.WriteInt32(int32(x))
		w.WriteInt32(int32(y))
		w.WriteInt32(int32(z))
		w.WriteUint8(0)
		c.Send(w.Bytes())
	case "bypass":
		w := wire.NewPacketWriter(clientpackets.OpcodeRequestBypassToServer)
		w.WriteString(expandRoles(st.Args[0], r.objs))
		c.Send(w.Bytes())
	case "link":
		w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
		w.WriteString(st.Args[0])
		c.Send(w.Bytes())
	case "hit":
		h := r.live[st.Args[0]]
		if h == nil || h.AlikeDead() {
			fail("NPC %s is not a live attackable", st.Args[0])
		}
		damage, _ := strconv.Atoi(st.Args[1])
		r.onPlayer(t, func(p attackable.Combatant) { h.TakeDamage(damage, p) })
	case "kill":
		n := 1
		if len(st.Args) == 2 {
			n, _ = killCount(st.Args[1])
		}
		role := st.Args[0]
		if r.live[role] == nil && r.objs[role] != 0 {
			fail("NPC %s is not an attackable", role)
		}
		for range n {
			if h := r.live[role]; h == nil || h.AlikeDead() {
				r.spawn(t, r.npcs[role])
			}
			h := r.live[role]
			r.onPlayer(t, func(p attackable.Combatant) { h.TakeDamage(h.MaxHP()+1, p) })
			r.srv.Advance(t, killWait)
		}
	case "advance":
		d, _ := time.ParseDuration(st.Args[0])
		r.srv.Advance(t, d)
	case "relog":
		c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
		r.readUntil(t, serverpackets.OpcodeCharSelectInfo)
		r.enterWorld(t)
	}
	frames := r.srv.ReadQueued(t, c)
	if st.Verb != "relog" {
		var got []string
		for _, f := range frames {
			line, ok, err := render(f, r.roles)
			if err != nil {
				fail("frame %x: %v", f, err)
			}
			if ok {
				got = append(got, line)
			}
		}
		want, err := expandPackets(st.Packets, r.texts)
		if err != nil {
			fail("%v", err)
		}
		if d := FirstDiff("packet", got, want); d != "" {
			fail("%s", d)
		}
	}
	r.srv.FlushPersistence(t)
	if d := FirstDiff("statement", statements(r.srv.TakeJournalWrites()), st.Statements); d != "" {
		fail("%s", d)
	}
	for _, ck := range st.Checks {
		if d := r.check(t, ck); d != "" {
			fail("%s (line %d)", d, ck.Line)
		}
	}
}

// onPlayer runs fn on the character's queue, where its hits land, and
// waits for it.
func (r *run) onPlayer(t *testing.T, fn func(p attackable.Combatant)) {
	t.Helper()
	obj, ok := r.srv.State.Player(r.player)
	if !ok {
		t.Fatal("the character is not in the world")
	}
	p, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("the character %T is not a combatant", obj)
	}
	done := make(chan struct{})
	if !r.srv.PlayerQueue(t, r.player).Post(func() {
		defer close(done)
		fn(p)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
}

// check runs one items, rows or state line, describing the first
// difference.
func (r *run) check(t *testing.T, ck Check) string {
	t.Helper()
	switch ck.Kind {
	case "items":
		var got, want []string
		for _, a := range ck.Args {
			id, count, _ := strings.Cut(a, "=")
			n, _ := strconv.ParseInt(id, 10, 32)
			got = append(got, fmt.Sprintf("%s=%d", id, r.srv.PlayerItemCount(t, r.player, int32(n))))
			want = append(want, id+"="+count)
		}
		return FirstDiff("item", got, want)
	case "rows":
		quest, want := ck.Args[0], slices.Clone(ck.Args[1:])
		if len(want) == 1 && want[0] == "-" {
			want = nil
		}
		slices.Sort(want)
		return FirstDiff(quest+" row", r.rows(t, quest), want)
	case "state":
		quest, want := ck.Args[0], ck.Args[1]
		got := "-"
		r.srv.RunQuest(t, r.player, quest, func(_ *script.Quests, c *player.Character, _ *script.Script) {
			if st := c.Quests().State(quest); st != nil {
				got = st.Status().String()
			}
		})
		return FirstDiff(quest+" state", []string{got}, []string{want})
	}
	return ""
}

// rows returns the character's character_quests rows of quest as
// var=value, sorted.
func (r *run) rows(t *testing.T, quest string) []string {
	t.Helper()
	rows, err := r.srv.DB.QueryContext(context.Background(),
		"SELECT var,value FROM character_quests WHERE charId=? AND name=? ORDER BY var", r.player, quest)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			t.Fatal(err)
		}
		out = append(out, k+"="+v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// enterWorld selects the character and enters the world.
func (r *run) enterWorld(t *testing.T) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	r.srv.Client.Send(w.Bytes())
	r.readUntil(t, serverpackets.OpcodeCharSelected)
	r.srv.Client.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	r.srv.ReadQueued(t, r.srv.Client)
}

// readUntil reads frames until one with opcode arrives.
func (r *run) readUntil(t *testing.T, opcode byte) {
	t.Helper()
	for range 100 {
		if f := r.srv.Client.Read(); f[0] == opcode {
			return
		}
	}
	t.Fatalf("no frame with opcode %#x within 100 frames", opcode)
}

// statements renders journal writes as Q lines.
func statements(writes []questlog.Write) []string {
	var out []string
	for _, w := range writes {
		switch w.Op {
		case questlog.OpSet:
			out = append(out, fmt.Sprintf("upsert %s | %s | %s", w.Quest, w.Var, w.Value))
		case questlog.OpUnset:
			out = append(out, fmt.Sprintf("delete-var %s | %s", w.Quest, w.Var))
		case questlog.OpDelete:
			out = append(out, "delete-quest "+w.Quest)
		case questlog.OpComplete:
			out = append(out, "delete-except-state "+w.Quest)
		}
	}
	return out
}

// loadPages reads the shipped pages the scenario serves, keyed by their
// path below data/html.
func loadPages(t *testing.T, paths []string) map[string]string {
	t.Helper()
	pages := map[string]string{}
	if len(paths) == 0 {
		return pages
	}
	root := datapack.Path(t, "data", "html")
	read := func(p string) {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		pages[p] = string(b)
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, "/") {
			read(p)
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				read(path.Join(p, e.Name()))
			}
		}
	}
	return pages
}

// pageTexts returns each page as the server shows it: loaded through the
// production page cache, which normalizes line ends.
func pageTexts(t *testing.T, pages map[string]string) map[string]string {
	t.Helper()
	texts := map[string]string{}
	if len(pages) == 0 {
		return texts
	}
	cache := gameservertest.HTMLCache(t, pages)
	for p := range pages {
		texts[p], _ = cache.Get(p)
	}
	return texts
}

// shipped is the shipped item table, loaded once per test binary.
var shipped struct {
	once  sync.Once
	table *item.Table
	err   error
}

// itemTemplates is the fixture item catalog with the shipped templates of
// ids added.
func itemTemplates(t *testing.T, ids []int32) *item.Table {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	if len(ids) == 0 {
		return item.NewTable(templates)
	}
	dir := datapack.Path(t, "data", "xml", "items")
	shipped.once.Do(func() { shipped.table, shipped.err = gamexml.LoadItemTemplates(dir, zerolog.Nop()) })
	if shipped.err != nil {
		t.Fatalf("load shipped items: %v", shipped.err)
	}
	for _, id := range ids {
		tmpl, ok := shipped.table.Get(id)
		if !ok {
			t.Fatalf("no shipped item %d", id)
		}
		templates = append(templates, tmpl)
	}
	return item.NewTable(templates)
}

// moduleRoot is the Go module's root directory.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no caller information")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}
