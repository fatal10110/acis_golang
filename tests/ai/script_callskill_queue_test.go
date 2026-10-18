package ai

import (
	"encoding/binary"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// selfTeleportSkillID is a fixture self-target TELEPORT skill.
const selfTeleportSkillID = 9301

// TestNPCHookCallSkillRunsOnTheCastersQueue pins where a script's skill
// call with a player caster runs when an NPC's hook asks it: on the
// player's own queue, once the hook's task is done, so the skill's effects
// (here a self teleport) never touch the player's move, cast or position
// while that queue runs them. The rest of the hook still reads the old
// position; the player then gets the cast's one MagicSkillUse, naming it
// as caster and target, followed by its TeleportToLocation, and stands at
// the skill's coordinates with its walk over.
func TestNPCHookCallSkillRunsOnTheCastersQueue(t *testing.T) {
	t.Parallel()
	dest := location.Location{X: 1000, Y: 2000, Z: -3000}
	def := modelskill.Definition{
		ID: selfTeleportSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "TELEPORT", HitTime: 500, TeleCoords: &dest,
	}
	db := sqltest.SharedDB(t)
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, def})
	w := bootWarriorsWith(t, []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
	}, warriorTemplate(warriorID, nil))
	h := w.spawn(t, warriorID, 1000, 0)
	walkTo := location.Location{X: w.at.X, Y: w.at.Y + 2000, Z: w.at.Z}
	w.walkStep(t, w.at, walkTo)
	x, y, z := w.player.Position()
	before := location.Location{X: x, Y: y, Z: z}
	if before == w.at {
		t.Fatal("test setup: the character did not start walking")
	}

	p := script.PlayerOf(w.player)
	var during location.Location
	done := make(chan struct{})
	if !h.Queue().Post(func() {
		defer close(done)
		(&script.Script{}).CallSkill(p, p, modelskill.Ref{ID: selfTeleportSkillID, Level: 1})
		x, y, z := w.player.Position()
		during = location.Location{X: x, Y: y, Z: z}
	}) {
		t.Fatal("the monster's queue is closed")
	}
	<-done
	if during != before {
		t.Fatalf("the hook read the character at %v after calling the skill, want its old %v", during, before)
	}
	w.srv.TickPositions()
	w.srv.Settle(t)
	frames := w.srv.ReadQueued(t, w.srv.Client)
	casts := framesOf(frames, serverpackets.OpcodeMagicSkillUse, w.player.ObjectID())
	if len(casts) != 1 {
		t.Fatalf("MagicSkillUse frames = %d, want 1", len(casts))
	}
	field := func(i int) int32 { return int32(binary.LittleEndian.Uint32(casts[0][1+4*i:])) }
	if target, id, level, hitTime := field(1), field(2), field(3), field(4); target != w.player.ObjectID() || id != selfTeleportSkillID || level != 1 || hitTime != 500 {
		t.Fatalf("MagicSkillUse target %d skill %d-%d hit time %d, want %d %d-1 500", target, id, level, hitTime, w.player.ObjectID(), selfTeleportSkillID)
	}
	teleports := framesOf(frames, serverpackets.OpcodeTeleportToLocation, w.player.ObjectID())
	if len(teleports) != 1 {
		t.Fatalf("TeleportToLocation frames = %d, want 1", len(teleports))
	}
	castAt, teleportAt := -1, -1
	for i, f := range frames {
		switch {
		case &f[0] == &casts[0][0]:
			castAt = i
		case &f[0] == &teleports[0][0]:
			teleportAt = i
		}
	}
	if castAt > teleportAt {
		t.Fatalf("MagicSkillUse at frame %d, after TeleportToLocation at %d", castAt, teleportAt)
	}
	near := func() bool {
		x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
		return abs(x-dest.X) <= 20 && abs(y-dest.Y) <= 20
	}
	if !near() {
		x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
		t.Fatalf("character at %d,%d, want within 20 of the skill's %d,%d", x, y, dest.X, dest.Y)
	}
	w.srv.TickPositions()
	if !near() {
		x, y, _ := w.srv.PlayerPosition(t, w.player.ObjectID())
		t.Fatalf("character walked on to %d,%d after the teleport, want it kept near %d,%d", x, y, dest.X, dest.Y)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
