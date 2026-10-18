package quest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/cabalbuffer"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// The Seven Signs priests (CabalBuffer): a Preacher of Doom and an Orator
// of Revelations, and the fighter versions of their curse and blessing.
const (
	cabalBufferPath = "script.feature.CabalBuffer"
	preacherID      = 31093
	oratorID        = 31094
	curseFighter    = 4361
	blessFighter    = 4364
)

// shippedSkillTable is the datapack's skill table, loaded once.
var shippedSkillTable struct {
	once sync.Once
	defs *modelskill.Table
	err  error
}

func shippedSkillDefs(t *testing.T) *modelskill.Table {
	t.Helper()
	dir := datapack.Path(t, "data", "xml", "skills")
	shippedSkillTable.once.Do(func() {
		shippedSkillTable.defs, shippedSkillTable.err = xmldata.LoadSkillDefinitions(dir, zerolog.Nop())
	})
	if shippedSkillTable.err != nil {
		t.Fatal(shippedSkillTable.err)
	}
	return shippedSkillTable.defs
}

// scriptDraws is a script random source answering queued draws in order,
// 0 once they run out, and recording the bound of every draw. Hooks draw
// on the NPCs' queues, so every field is guarded by mu.
type scriptDraws struct {
	mu     sync.Mutex
	next   []int
	bounds []int
}

func (d *scriptDraws) rand(n int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bounds = append(d.bounds, n)
	if len(d.next) == 0 {
		return 0
	}
	v := d.next[0]
	d.next = d.next[1:]
	return v
}

func (d *scriptDraws) queue(draws ...int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.next = append(d.next, draws...)
}

// taken returns the bounds drawn since the last call.
func (d *scriptDraws) taken() []int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := d.bounds
	d.bounds = nil
	return out
}

// cabalWorld is a character signed up for cabal (none for NoCabal) while
// Dawn leads the Seven Signs, in the world with the priests' script, the AI
// task driven by hand.
type cabalWorld struct {
	*dialogWorld
	draws *scriptDraws
}

func bootCabalWorld(t *testing.T, cabal sevensigns.Cabal) *cabalWorld {
	t.Helper()
	draws := &scriptDraws{}
	w := bootSeamQuest(t, 20, nil, func(srv *gameservertest.Server, objID int32) {
		if cabal != sevensigns.NoCabal {
			if err := srv.SevenSigns.SetPlayerInfo(context.Background(), objID, cabal, sevensigns.Gnosis); err != nil {
				t.Fatal(err)
			}
		}
		srv.SevenSigns.AddFestivalScore(sevensigns.Dawn, 100)
	},
		gameservertest.WithAITask(),
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{preacherID: script.KindFolk, oratorID: script.KindFolk},
			[]script.Listing{{Path: cabalBufferPath}}, script.Catalog{cabalBufferPath: cabalbuffer.New}),
	)
	return &cabalWorld{dialogWorld: w, draws: draws}
}

// spawnPriest places the priest npcID 100 east of the character, casting
// the shipped skills, and lets its AI run a full cycle past its first
// tick.
func (w *cabalWorld) spawnPriest(t *testing.T, role string, npcID int) *npc.Folk {
	t.Helper()
	f := w.srv.SpawnCastingFolkNPCAt(t, priestTemplate(npcID),
		location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}, shippedSkillDefs(t))
	w.roles[f.ObjectID()] = role
	return f
}

// priestTemplate is the fixture civilian template with the shipped
// priests' caster stats (npcs/31000-31099.xml): what a curse's landing
// roll reads.
func priestTemplate(npcID int) *npc.Template {
	tmpl := gameservertest.FolkTemplate("Folk", npcID)
	tmpl.MPMax, tmpl.MAtk, tmpl.MDef, tmpl.PAtk, tmpl.PDef = 1493, 468, 206.28, 683.28, 296.1
	tmpl.INT, tmpl.WIT, tmpl.MEN = 21, 20, 10
	return tmpl
}

// priestAct is what a priest did over some AI ticks: its chat lines and
// the skills it cast on the character.
type priestAct struct {
	says  []string
	casts []msu
}

// tick runs n AI ticks and returns what the priest f said and cast.
func (w *cabalWorld) tick(t *testing.T, f *npc.Folk, n int) priestAct {
	t.Helper()
	var act priestAct
	for range n {
		if err := w.srv.AI.Tick(); err != nil {
			t.Fatal(err)
		}
		for _, fr := range framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay, serverpackets.OpcodeMagicSkillUse) {
			if fr.opcode == serverpackets.OpcodeMagicSkillUse {
				if m := readMSU(fr); m.caster == f.ObjectID() {
					act.casts = append(act.casts, m)
				}
				continue
			}
			obj, _, _, line := fr.r.ReadInt32(), fr.r.ReadInt32(), fr.r.ReadInt32(), fr.r.ReadString()
			if obj == f.ObjectID() {
				act.says = append(act.says, line)
			}
		}
	}
	return act
}

// leaveAndReturn takes the character out of the priests' sight and back,
// so their next scan sees it again.
func (w *cabalWorld) leaveAndReturn(t *testing.T, f *npc.Folk) priestAct {
	t.Helper()
	pl, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player missing from the world")
	}
	if err := w.srv.State.Move(pl, w.at.X, w.at.Y-2000, w.at.Z); err != nil {
		t.Fatal(err)
	}
	w.tick(t, f, 1)
	if err := w.srv.State.Move(pl, w.at.X, w.at.Y, w.at.Z); err != nil {
		t.Fatal(err)
	}
	return w.tick(t, f, 3)
}

// heldEffect reports whether the character holds an effect of skillID.
func (w *cabalWorld) heldEffect(t *testing.T, skillID int32) bool {
	t.Helper()
	var held bool
	w.srv.RunScript(t, w.player, "CabalBuffer", func(_ *script.Script, p *script.Player) { held = p.HasEffect(skillID) })
	return held
}

func npcLine(t *testing.T, id int32, name string) string {
	t.Helper()
	text, ok := npcstring.Text(id)
	if !ok {
		t.Fatalf("no NpcString %d", id)
	}
	return strings.Replace(text, "%s", name, 1)
}

// TestCabalBufferPreacherCursesTheLosingCabal (CabalBuffer.onSeeCreature):
// Dawn leads, so a Dusk fighter seen by a Preacher of Doom, holding no
// curse, is cursed with the fighter curse at level 1, the rare line naming
// him said first: the draw of 10000 came out 0. The sighting draws 100
// then 10000. (Whether the curse lands is the debuff's own landing roll.)
func TestCabalBufferPreacherCursesTheLosingCabal(t *testing.T) {
	t.Parallel()
	w := bootCabalWorld(t, sevensigns.Dusk)
	preacher := w.spawnPriest(t, "preacher", preacherID)

	w.draws.queue(50, 0)
	act := w.tick(t, preacher, 4)
	if want := []string{npcLine(t, 1000420, "Seeker")}; !slices.Equal(act.says, want) {
		t.Fatalf("preacher says %q, want %q", act.says, want)
	}
	if len(act.casts) != 1 || act.casts[0].target != w.player || act.casts[0].skill != curseFighter || act.casts[0].level != 1 {
		t.Fatalf("preacher casts %+v, want the fighter curse level 1 on the character", act.casts)
	}
	if got := w.draws.taken(); !slices.Equal(got, []int{100, 10000}) {
		t.Fatalf("draws = %v, want 100 then 10000", got)
	}
}

// TestCabalBufferOratorBlessesTheWinningCabal: a Dawn fighter seen by an
// Orator of Revelations, holding no blessing, is blessed with the fighter
// blessing at level 1, with no line as the draw of 10000 is not 0. Seen
// again while blessed, he is blessed at level 2 when the draw of 100 is
// under 5, with the second line naming him when the draw of 10000 is under
// 500; with the draw of 100 at 5 nothing happens. A Preacher of Doom
// leaves him alone. Every sighting draws 100 then 10000.
func TestCabalBufferOratorBlessesTheWinningCabal(t *testing.T) {
	t.Parallel()
	w := bootCabalWorld(t, sevensigns.Dawn)
	orator := w.spawnPriest(t, "orator", oratorID)

	w.draws.queue(50, 1)
	act := w.tick(t, orator, 4)
	if len(act.says) != 0 {
		t.Fatalf("orator says %q, want nothing", act.says)
	}
	if len(act.casts) != 1 || act.casts[0].target != w.player || act.casts[0].skill != blessFighter || act.casts[0].level != 1 {
		t.Fatalf("orator casts %+v, want the fighter blessing level 1 on the character", act.casts)
	}
	if got := w.draws.taken(); !slices.Equal(got, []int{100, 10000}) {
		t.Fatalf("draws = %v, want 100 then 10000", got)
	}
	w.srv.AdvanceUntil(t, "the blessing landing", func() bool { return w.heldEffect(t, blessFighter) })
	w.srv.AdvanceUntil(t, "the orator's cast ending", func() bool { return !orator.CastingNow() })

	w.draws.queue(4, 499)
	act = w.leaveAndReturn(t, orator)
	if want := []string{npcLine(t, 1000424, "Seeker")}; !slices.Equal(act.says, want) {
		t.Fatalf("orator says to the blessed character %q, want %q", act.says, want)
	}
	if len(act.casts) != 1 || act.casts[0].skill != blessFighter || act.casts[0].level != 2 {
		t.Fatalf("orator casts on the blessed character %+v, want the fighter blessing level 2", act.casts)
	}
	if got := w.draws.taken(); !slices.Equal(got, []int{100, 10000}) {
		t.Fatalf("draws = %v, want 100 then 10000", got)
	}

	w.draws.queue(5, 0)
	if act = w.leaveAndReturn(t, orator); len(act.says) != 0 || len(act.casts) != 0 {
		t.Fatalf("orator with the draw of 100 at 5 did %+v, want nothing", act)
	}
	if got := w.draws.taken(); !slices.Equal(got, []int{100, 10000}) {
		t.Fatalf("draws = %v, want 100 then 10000", got)
	}

	preacher := w.spawnPriest(t, "preacher", preacherID)
	if act = w.tick(t, preacher, 4); len(act.says) != 0 || len(act.casts) != 0 {
		t.Fatalf("preacher with a Dawn character did %+v, want nothing", act)
	}
	if got := w.draws.taken(); !slices.Equal(got, []int{100, 10000}) {
		t.Fatalf("draws = %v, want 100 then 10000", got)
	}
}

// TestCabalBufferLeavesPlayersOfNoCabalAlone: a character signed up for no
// cabal is left alone by both priests, which draw nothing for him.
func TestCabalBufferLeavesPlayersOfNoCabalAlone(t *testing.T) {
	t.Parallel()
	w := bootCabalWorld(t, sevensigns.NoCabal)
	preacher := w.spawnPriest(t, "preacher", preacherID)
	orator := w.spawnPriest(t, "orator", oratorID)

	if act := w.tick(t, preacher, 4); len(act.says) != 0 || len(act.casts) != 0 {
		t.Fatalf("preacher did %+v, want nothing", act)
	}
	if act := w.leaveAndReturn(t, orator); len(act.says) != 0 || len(act.casts) != 0 {
		t.Fatalf("orator did %+v, want nothing", act)
	}
	if got := w.draws.taken(); len(got) != 0 {
		t.Fatalf("draws = %v, want none", got)
	}
}

// priestsSpawnlist places a Preacher of Doom and an Orator of Revelations
// beside the character, each the one NPC of its npcmaker.
const priestsSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="town" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="town_preacher" territory="town" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31093" total="1" pos="70;20;30;0" respawn="60sec"/>
	</npcmaker>
	<npcmaker name="town_orator" territory="town" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="31094" total="1" pos="10;80;30;0" respawn="60sec"/>
	</npcmaker>
</list>`

// liveFolk returns the one civilian NPC of template id in the world.
func liveFolk(t *testing.T, srv *gameservertest.Server, id int) *npc.Folk {
	t.Helper()
	var found []*npc.Folk
	for _, obj := range srv.State.Objects() {
		if f, ok := obj.(*npc.Folk); ok && f.Instance.Template.ID == id {
			found = append(found, f)
		}
	}
	if len(found) != 1 {
		t.Fatalf("live civilian NPCs of template %d = %d, want 1", id, len(found))
	}
	return found[0]
}

// TestCabalBufferPriestsPreachEveryMinute (CabalBuffer.onCreated,
// onTimer, onDecayed): each priest its npcmaker places starts preaching
// as it appears, a minute later and then every minute, a line of its own
// drawn from four. A priest that leaves the world stops preaching; the
// other goes on.
func TestCabalBufferPriestsPreachEveryMinute(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "priests.xml"), []byte(priestsSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := xmldata.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	draws := &scriptDraws{}
	draws.queue(3, 3)
	w := bootSeamQuest(t, 20, nil, nil,
		gameservertest.WithScriptRand(draws.rand),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			priestTemplate(preacherID), priestTemplate(oratorID),
		})),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{preacherID: script.KindFolk, oratorID: script.KindFolk},
			[]script.Listing{{Path: cabalBufferPath}}, script.Catalog{cabalBufferPath: cabalbuffer.New}),
	)
	preacher, orator := liveFolk(t, w.srv, preacherID), liveFolk(t, w.srv, oratorID)
	w.srv.ReadQueued(t, w.srv.Client)
	says := func(t *testing.T) map[int32][]string {
		t.Helper()
		out := map[int32][]string{}
		for _, fr := range framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeNpcSay) {
			obj, _, _, line := fr.r.ReadInt32(), fr.r.ReadInt32(), fr.r.ReadInt32(), fr.r.ReadString()
			out[obj] = append(out[obj], line)
		}
		return out
	}
	want := func(t *testing.T, step string, got map[int32][]string, preach map[int32]int32) {
		t.Helper()
		if len(got) != len(preach) {
			t.Fatalf("%s: priests said %q, want %d of them to preach", step, got, len(preach))
		}
		for obj, id := range preach {
			if line := npcLine(t, id, ""); len(got[obj]) != 1 || got[obj][0] != line {
				t.Fatalf("%s: priest %d said %q, want %q", step, obj, got[obj], line)
			}
		}
	}

	w.srv.Advance(t, 59*time.Second)
	if got := says(t); len(got) != 0 {
		t.Fatalf("priests said %q within their first minute, want nothing", got)
	}
	w.srv.Advance(t, time.Second)
	want(t, "after a minute", says(t), map[int32]int32{preacher.ObjectID(): 1000417, orator.ObjectID(): 1000423})
	if got := draws.taken(); !slices.Equal(got, []int{4, 4}) {
		t.Fatalf("draws = %v, want one of 4 per priest", got)
	}

	w.srv.Advance(t, 60*time.Second)
	want(t, "after two minutes", says(t), map[int32]int32{preacher.ObjectID(): 1000303, orator.ObjectID(): 1000305})

	preacher.DeleteNow()
	w.srv.Advance(t, 60*time.Second)
	want(t, "after the preacher left", says(t), map[int32]int32{orator.ObjectID(): 1000305})
}
