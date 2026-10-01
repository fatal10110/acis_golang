package npcs

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// arenaManager is one of the two NPCs Npc.onBypassFeedback's CPRecovery
// answers, with the shipped page that offers the paid restore.
type arenaManager struct {
	name string
	kind string
	id   int
	page string // under data/html
}

var (
	// arenaDirector is a plain Folk (data/html/default/31226.htm).
	arenaDirector = arenaManager{"31226 Folk", "Folk", 31226, "default/31226.htm"}
	// arenaWarehouse is a WarehouseKeeper, whose own bypass handler runs
	// first (data/html/warehouse/31225.htm).
	arenaWarehouse = arenaManager{"31225 WarehouseKeeper", "WarehouseKeeper", 31225, "warehouse/31225.htm"}
)

// arenaCPRecoverySkill is the skill the arena manager casts (4380, Arena CP
// Recovery: COMBATPOINTHEAL, power 5000, target ONE, castRange 600).
const arenaCPRecoverySkill = 4380

// cpRecoveryWorld enters the world holding adena next to the shipped arena
// director, with skill data loaded and the AI task driven by hand. The
// player's CP is emptied so a restore shows.
func cpRecoveryWorld(t *testing.T, adena int32) (*folkWorld, *npc.Folk) {
	t.Helper()
	return cpRecoveryWorldAt(t, arenaDirector, adena)
}

// cpRecoveryWorldAt is cpRecoveryWorld next to manager.
func cpRecoveryWorldAt(t *testing.T, manager arenaManager, adena int32) (*folkWorld, *npc.Folk) {
	t.Helper()
	page, err := os.ReadFile(datapack.Path(t, "data", "html", manager.page))
	if err != nil {
		t.Fatalf("read arena manager page: %v", err)
	}
	pages := dialogPages()
	pages[manager.page] = string(page)
	defs, err := xmldata.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHTMLPages(pages),
		gameservertest.WithAITask(),
		noBypassReuse,
	)
	w := &folkWorld{srv: srv, c: srv.Client, player: srv.SoleObjectID(t)}
	srv.GiveItem(t, w.player, item.AdenaID, adena)
	startInWorld(t, w.c)
	x, y, z := srv.PlayerPosition(t, w.player)
	w.at = location.Location{X: x, Y: y, Z: z}
	arena := w.srv.SpawnCastingFolkNPCAt(t, folkTemplate(manager.kind, manager.id),
		location.Location{X: w.at.X + 60, Y: w.at.Y, Z: w.at.Z}, defs)
	w.settleAI(t)
	w.onPlayer(t, func(pc *player.Character) { pc.SetCP(0) })
	drainUntilQuiet(t, w.c)
	return w, arena
}

// onPlayer runs fn on the player's own queue and waits for it.
func (w *folkWorld) onPlayer(t *testing.T, fn func(*player.Character)) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatal("post to player queue: queue closed")
	}
	<-done
}

// cp reads the player's current and maximum CP.
func (w *folkWorld) cp(t *testing.T) (cur, maxCP float64) {
	t.Helper()
	w.onPlayer(t, func(pc *player.Character) { cur, maxCP = pc.CP(), pc.MaxCPValue() })
	return cur, maxCP
}

// dialogFrames sends command and keeps the frames a dialog command is
// judged by: pages, messages and releases.
func (w *folkWorld) dialogFrames(t *testing.T, command string) [][]byte {
	t.Helper()
	var out [][]byte
	for _, f := range w.bypass(t, command) {
		switch f[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed:
			out = append(out, f)
		}
	}
	return out
}

// tickAI runs one AI cycle and returns what the client then receives.
func (w *folkWorld) tickAI(t *testing.T) [][]byte {
	t.Helper()
	if err := w.srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() = %v", err)
	}
	return drainFrames(t, w.c)
}

// settleAI runs one full three-tick AI cycle, dropping what it shows: a
// civilian NPC spawned just before has lived past the first tick its AI
// acts on nothing on, gone idle to its walk stance unless it walks a route
// (NpcAI.runAI), and meets the decay of a desire queued next on its third
// tick, as on a server that has run for a while.
func (w *folkWorld) settleAI(t *testing.T) {
	t.Helper()
	for range 3 {
		w.tickAI(t)
	}
}

func textParam(s string) []byte {
	out := le32(serverpackets.SystemMessageParamText)
	for _, r := range s {
		out = binary.LittleEndian.AppendUint16(out, uint16(r))
	}
	return append(out, 0, 0)
}

// TestBypassCPRecoveryArenaManagerCastsRestore pins Npc.onBypassFeedback's
// CPRecovery at an arena manager: from its shipped page, the talker pays
// 100 adena (S1_DISAPPEARED_ADENA), is told S1_CP_WILL_BE_RESTORED with
// their own name, and is released by the dispatcher. On the NPC's next AI
// tick it casts 4380 on the talker for everyone watching (MagicSkillUse,
// MagicSkillLaunched onto the talker), the talker's CP is restored
// (CombatPointHeal: S1_CP_WILL_BE_RESTORED with the amount), and it does
// not cast again. Both arena managers do so: 31226, a plain Folk, and
// 31225, a WarehouseKeeper whose own handler lets CPRecovery through to
// Npc's.
func TestBypassCPRecoveryArenaManagerCastsRestore(t *testing.T) {
	t.Parallel()
	for _, manager := range []arenaManager{arenaDirector, arenaWarehouse} {
		t.Run(manager.name, func(t *testing.T) {
			t.Parallel()
			testCPRecoveryCastsRestore(t, manager)
		})
	}
}

func testCPRecoveryCastsRestore(t *testing.T, manager arenaManager) {
	w, arena := cpRecoveryWorldAt(t, manager, 500)
	w.talkTo(t, arena)

	assertFrames(t, "CPRecovery", w.dialogFrames(t, npcCommand(arena, "CPRecovery")),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(100)),
		sysMsg(serverpackets.SystemMessageS1CPWillBeRestored, textParam("Talker")),
		[]byte{serverpackets.OpcodeActionFailed},
	)
	if got := w.held(t, item.AdenaID); got != 400 {
		t.Fatalf("adena held = %d, want 400", got)
	}
	if got := w.savedCount(t, item.AdenaID); got != 400 {
		t.Fatalf("adena saved = %d, want 400", got)
	}

	frames := w.tickAI(t)
	use, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillUse)
	if !ok {
		t.Fatalf("AI tick frames %x, want the arena manager's MagicSkillUse", opcodes(frames))
	}
	r := wire.NewReader(use[1:])
	caster, target, skillID, level, hitTime, reuse := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	if caster != arena.ObjectID() || target != w.player || skillID != arenaCPRecoverySkill || level != 1 || hitTime != 0 || reuse != 0 {
		t.Fatalf("MagicSkillUse = caster %d target %d skill %d/%d hit %d reuse %d, want %d %d %d/1 0 0",
			caster, target, skillID, level, hitTime, reuse, arena.ObjectID(), w.player, arenaCPRecoverySkill)
	}
	launched, ok := firstOpcode(frames, serverpackets.OpcodeMagicSkillLaunched)
	if !ok {
		t.Fatalf("AI tick frames %x, want MagicSkillLaunched", opcodes(frames))
	}
	r = wire.NewReader(launched[1:])
	if caster, skillID, level, n, onto := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != arena.ObjectID() || skillID != arenaCPRecoverySkill || level != 1 || n != 1 || onto != w.player {
		t.Fatalf("MagicSkillLaunched = caster %d skill %d/%d onto %d x %d, want %d %d/1 onto %d", caster, skillID, level, n, onto, arena.ObjectID(), arenaCPRecoverySkill, w.player)
	}

	cur, maxCP := w.cp(t)
	if cur != maxCP {
		t.Fatalf("CP = %v, want restored to its %v maximum", cur, maxCP)
	}
	// The CP set shows the talker its status, then the amount restored.
	wantOrder := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillLaunched, serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeSystemMessage}
	if got := opcodes(frames); string(got) != string(wantOrder) {
		t.Fatalf("AI tick frames %x, want %x", got, wantOrder)
	}
	if restored := sysMsg(serverpackets.SystemMessageS1CPWillBeRestored, numberParam(int32(maxCP))); string(frames[3]) != string(restored) {
		t.Fatalf("restore message = %x, want %x", frames[3], restored)
	}

	if again := w.tickAI(t); len(again) != 0 {
		t.Fatalf("next AI tick frames %x, want none: the cast closed its desire", opcodes(again))
	}
}

// TestBypassCPRecoveryWithoutFee pins the refusal: a talker holding less
// than 100 adena is told YOU_NOT_ENOUGH_ADENA and released; nothing is
// taken and the NPC casts nothing.
func TestBypassCPRecoveryWithoutFee(t *testing.T) {
	t.Parallel()
	w, arena := cpRecoveryWorld(t, 99)
	w.talkTo(t, arena)

	assertFrames(t, "CPRecovery", w.dialogFrames(t, npcCommand(arena, "CPRecovery")),
		sysMsg(serverpackets.SystemMessageYouNotEnoughAdena),
		[]byte{serverpackets.OpcodeActionFailed},
	)
	if got := w.savedCount(t, item.AdenaID); got != 99 {
		t.Fatalf("adena saved = %d, want 99", got)
	}
	if frames := w.tickAI(t); len(frames) != 0 {
		t.Fatalf("AI tick frames %x, want none", opcodes(frames))
	}
	if cur, _ := w.cp(t); cur != 0 {
		t.Fatalf("CP = %v, want 0", cur)
	}
}

// TestBypassCPRecoveryOnlyAtArenaManagers pins the NPC id gate: any other
// NPC answers CPRecovery with the dispatcher's ActionFailed alone, takes
// nothing and casts nothing.
func TestBypassCPRecoveryOnlyAtArenaManagers(t *testing.T) {
	t.Parallel()
	w, _ := cpRecoveryWorld(t, 500)
	defs := modelskill.NewTable(nil)
	plain := w.srv.SpawnCastingFolkNPCAt(t, folkTemplate("Folk", 31227),
		location.Location{X: w.at.X - 60, Y: w.at.Y, Z: w.at.Z}, defs)
	drainUntilQuiet(t, w.c)

	w.openAnyNpcPage(t)
	assertAnswer(t, w.dialogFrames(t, npcCommand(plain, "CPRecovery")), releaseOnly, plain, "")
	if got := w.savedCount(t, item.AdenaID); got != 500 {
		t.Fatalf("adena saved = %d, want 500", got)
	}
	if frames := w.tickAI(t); len(frames) != 0 {
		t.Fatalf("AI tick frames %x, want none", opcodes(frames))
	}
}
