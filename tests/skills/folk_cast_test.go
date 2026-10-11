package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	folkAuraHealSkill = modelskill.ID(9201)
	folkLongBuffSkill = modelskill.ID(9202)
)

// bootFolkCaster brings a player in next to a civilian NPC with the
// production cast runtime over defs, the AI task driven by hand and
// settled.
func bootFolkCaster(t *testing.T, defs []modelskill.Definition) (*gameservertest.Server, int32, *npc.Folk) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAITask(),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	folk := srv.SpawnCastingFolkNPCAt(t, gameservertest.FolkTemplate("Folk", 30100),
		location.Location{X: x + 40, Y: y, Z: z}, modelskill.NewTable(defs))
	settleAI(t, srv)
	drainUntilQuiet(t, c)
	return srv, objID, folk
}

// addCastDesire queues a cast desire on folk the way a dialog command or a
// script does, and waits for the NPC's queue to take it.
func addCastDesire(t *testing.T, folk *npc.Folk, ref modelskill.Ref) {
	t.Helper()
	folk.AddCastDesire(folk, ref, 1_000_000, true, true)
	done := make(chan struct{})
	if !folk.Queue().Post(func() { close(done) }) {
		t.Fatal("post to folk queue: queue closed")
	}
	<-done
}

// tickAI runs one AI cycle and returns what the client then receives.
func tickAI(t *testing.T, srv *gameservertest.Server) [][]byte {
	t.Helper()
	if err := srv.AI.Tick(); err != nil {
		t.Fatalf("AI.Tick() = %v", err)
	}
	return readUntilQuiet(t, srv.Client)
}

// settleAI runs one full three-tick AI cycle, dropping what it shows: a
// civilian NPC spawned just before has lived past the first tick its AI
// acts on nothing on and gone idle to its walk stance (NpcAI.runAI), as on
// a server that has run for a while.
func settleAI(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	for range 3 {
		tickAI(t, srv)
	}
}

// TestFolkAuraCastLandsOnPlayersOnly pins TargetAura.java for a live Folk
// cast: a cast desire for an AURA heal runs on the NPC's next AI tick, its
// MagicSkillLaunched names the nearby player alone, and the heal lands on
// that player but not on the wounded monster standing as close.
func TestFolkAuraCastLandsOnPlayersOnly(t *testing.T) {
	t.Parallel()
	srv, objID, folk := bootFolkCaster(t, []modelskill.Definition{{
		ID: folkAuraHealSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetAura, Radius: 200, SkillType: "HEAL", Power: 50,
		StaticHitTime: true, StaticReuse: true,
	}})
	c := srv.Client
	x, y, z := srv.PlayerPosition(t, objID)
	monster := srv.SpawnHostileNPCAt(t, location.Location{X: x + 80, Y: y, Z: z})
	onNPCQueue(t, monster, func() { monster.SetCurrentHP(monster.MaxHP() / 2) })
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.SetHP(1) })
	drainUntilQuiet(t, c)
	monsterHP := monster.CurrentHP()

	addCastDesire(t, folk, modelskill.Ref{ID: folkAuraHealSkill, Level: 1})
	frames := tickAI(t, srv)

	if use := framesWith(frames, serverpackets.OpcodeMagicSkillUse, folk.ObjectID()); len(use) != 1 {
		t.Fatalf("AI tick frames %x, want one MagicSkillUse", frameOpcodes(frames))
	}
	launched := framesWith(frames, serverpackets.OpcodeMagicSkillLaunched, folk.ObjectID())
	if len(launched) != 1 {
		t.Fatalf("AI tick frames %x, want one MagicSkillLaunched", frameOpcodes(frames))
	}
	r := wire.NewReader(launched[0][1:])
	if caster, skillID, _, n, onto := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != folk.ObjectID() || skillID != int32(folkAuraHealSkill) || n != 1 || onto != objID {
		t.Fatalf("MagicSkillLaunched = caster %d skill %d onto %d x %d, want %d %d onto the player %d alone", caster, skillID, n, onto, folk.ObjectID(), folkAuraHealSkill, objID)
	}
	var hp float64
	onPlayerQueue(t, srv, objID, func(pc *player.Character) { hp = pc.HP() })
	if hp <= 1 {
		t.Fatalf("player HP = %v, want healed above 1", hp)
	}
	if got := monster.CurrentHP(); got != monsterHP {
		t.Fatalf("monster HP = %d, want %d: a Folk's aura passes over NPCs", got, monsterHP)
	}
}

// TestFolkCastAbortCancelsAndDropsItsDesire pins the abort a Folk's death
// or despawn uses: a cast in flight is canceled for its observers
// (MagicSkillCanceled) and the desire behind it is gone, so the next AI
// tick casts nothing.
func TestFolkCastAbortCancelsAndDropsItsDesire(t *testing.T) {
	t.Parallel()
	srv, _, folk := bootFolkCaster(t, []modelskill.Definition{{
		ID: folkLongBuffSkill, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetSelf, SkillType: "BUFF", HitTime: 2000,
		StaticHitTime: true, StaticReuse: true,
	}})
	c := srv.Client

	addCastDesire(t, folk, modelskill.Ref{ID: folkLongBuffSkill, Level: 1})
	frames := tickAI(t, srv)
	if use := framesWith(frames, serverpackets.OpcodeMagicSkillUse, folk.ObjectID()); len(use) != 1 {
		t.Fatalf("AI tick frames %x, want the Folk's MagicSkillUse", frameOpcodes(frames))
	}
	if !folk.CastingNow() {
		t.Fatal("Folk not casting after its MagicSkillUse")
	}

	folk.AbortCast()
	canceled := readUntilQuiet(t, c)
	if got := framesWith(canceled, serverpackets.OpcodeMagicSkillCanceled, folk.ObjectID()); len(got) != 1 {
		t.Fatalf("abort frames %x, want the Folk's MagicSkillCanceled", frameOpcodes(canceled))
	}
	if folk.CastingNow() {
		t.Fatal("Folk still casting after the abort")
	}
	if again := tickAI(t, srv); len(framesWith(again, serverpackets.OpcodeMagicSkillUse, folk.ObjectID())) != 0 {
		t.Fatalf("next AI tick frames %x, want no recast", frameOpcodes(again))
	}
}
