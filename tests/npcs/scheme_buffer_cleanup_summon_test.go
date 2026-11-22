package npcs

import (
	"encoding/binary"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSchemeBufferCleanupRefreshesSummonWindow: cleanup with a buffed
// summon out strips it and shows its owner the pet window again, then its
// now empty icon list, both before the page; the window clears the icons
// on the client, so the list follows it.
func TestSchemeBufferCleanupRefreshesSummonWindow(t *testing.T) {
	t.Parallel()
	buffer := schemebuffer.New(schemebuffer.DefaultConfig(), schemeBufferBuffs(t), schemeBufferSkills(t))
	const servitorNpcID = 14001
	w, f := schemeBufferWorld(t, buffer, 3000, gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
		ID: servitorNpcID, TemplateID: servitorNpcID, Type: "Servitor", Name: "Servitor", Level: 44,
		HPMax: 500, MPMax: 200, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}})))
	buffer.Restore([]schemebuffer.Row{{OwnerID: w.player, Name: "Pet", Skills: "1035,1036"}}, zerolog.Nop())
	w.onPlayer(t, func(pc *player.Character) {
		pc.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON"}.WithExtra(modelskill.Extra{NpcID: servitorNpcID, SummonTotalLifeTime: 1_200_000}))
	})
	var servitor *summon.Actor
	w.srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := w.srv.State.Summon(w.player)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	drainUntilQuiet(t, w.c)
	w.talkTo(t, f)
	w.bypass(t, npcCommand(f, "support"))
	w.bypass(t, npcCommand(f, "givebuffs Pet 1000 pet"))
	w.bypass(t, npcCommand(f, "menu"))
	drainUntilQuiet(t, w.c)

	frames := w.bypass(t, npcCommand(f, "cleanup"))
	assertSchemePage(t, "cleanup", frames[len(frames)-2:], datapackPage(t, f, "50002.htm"))
	objectID := func(frame []byte) int32 { return int32(binary.LittleEndian.Uint32(frame[5:9])) }
	// The icon list may also follow each buff as it ends; the one that
	// counts is the list right after the window.
	window := -1
	for i, frame := range frames[:len(frames)-2] {
		if frame[0] == serverpackets.OpcodePetInfo {
			if window >= 0 || objectID(frame) != servitor.ObjectID() {
				t.Fatalf("cleanup = %x, want one PetInfo, for the servitor", opcodes(frames))
			}
			window = i
		}
	}
	if window < 0 || window+1 >= len(frames)-2 {
		t.Fatalf("cleanup = %x, want PetInfo then the summon's PartySpelled before the page", opcodes(frames))
	}
	icons := frames[window+1]
	if icons[0] != serverpackets.OpcodePartySpelled || objectID(icons) != servitor.ObjectID() {
		t.Fatalf("cleanup = %x, want the summon's PartySpelled right after PetInfo", opcodes(frames))
	}
	if n := binary.LittleEndian.Uint32(icons[9:13]); n != 0 {
		t.Fatalf("summon icon list after the window holds %d effects, want none", n)
	}
}
