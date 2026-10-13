package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The Scroll of Resurrection and its skill.
const (
	resurrectionScrollID      int32 = 737
	resurrectionScrollSkillID int32 = 2014
)

// resurrectionScrollSkill is the scroll's resurrection of a dead player.
func resurrectionScrollSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(resurrectionScrollSkillID), Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetCorpsePlayer, SkillType: "RESURRECT", Power: 20,
		CastRange: 400, EffectRange: 600, HitTime: 3000, StaticHitTime: true,
		ItemConsumeID: int(resurrectionScrollID), ItemConsumeCount: 1,
	}
}

// resurrectionScrollTemplates is the behavior catalog plus the Scroll of
// Resurrection.
func resurrectionScrollTemplates() gameservertest.Option {
	return gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: resurrectionScrollID, Name: "Scroll of Resurrection", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true, Depositable: true,
		EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ScrollsOfResurrection", SharedReuseGroup: -1},
		AttachedSkills: []item.SkillRef{{ID: resurrectionScrollSkillID, Level: 1}},
	})))
}

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// TestResurrectionScrollOnBesiegedBattlefield has the Caster use a Scroll
// of Resurrection on the Stranger, dead on Gludio's battlefield while its
// siege runs (ScrollsOfResurrection.java:33-37): a Stranger with no siege
// state of its own, its clan on no side, is refused with
// CANNOT_BE_RESURRECTED_DURING_SIEGE alone and the scroll kept; one whose
// clan fights the siege is resurrected, the scroll's skill cast on it.
func TestResurrectionScrollOnBesiegedBattlefield(t *testing.T) {
	t.Parallel()
	damage := targetedSkill(socialSkillID, modelskill.TargetOne, true)
	damage.SkillType = "MDAM"
	for _, tt := range []struct {
		name    string
		rivals  string
		refused bool
	}{
		{name: "stranger fighting the siege", rivals: "ATTACKER"},
		{name: "stranger without a clan", refused: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSiegeSideScene(t, damage, "ATTACKER", tt.rivals, []modelskill.Definition{resurrectionScrollSkill()},
				map[int32]int32{resurrectionScrollID: 1}, resurrectionScrollTemplates())
			killer, ok := s.srv.State.Player(s.casterID)
			if !ok {
				t.Fatal("Caster missing from the world")
			}
			obj, ok := s.srv.State.Player(s.strangerID)
			if !ok {
				t.Fatal("Stranger missing from the world")
			}
			stranger, ok := network.OnlineCharacter(obj)
			if !ok {
				t.Fatalf("Stranger is %T", obj)
			}
			done := make(chan struct{})
			s.srv.PlayerQueue(t, s.strangerID).Post(func() {
				defer close(done)
				if !stranger.Kill(killer.(attackable.Combatant)) {
					t.Error("the Stranger did not die")
				}
			})
			<-done
			drainUntilQuiet(t, s.caster)
			drainUntilQuiet(t, s.stranger)

			s.caster.Send(encodeUseItem(s.held[resurrectionScrollID]))
			if tt.refused {
				assertStaticSystemMessage(t, s.caster.Read(), serverpackets.SystemMessageCannotBeResurrectedDuringSiege)
				if extra := collectUntilQuiet(t, s.caster); len(extra) != 0 {
					t.Fatalf("refused scroll answered more: %x", opcodes(extra))
				}
				return
			}
			frame := s.caster.Read()
			assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "scroll MagicSkillUse")
			r := wire.NewReader(frame[1:])
			if caster, target, skill := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != s.casterID || target != s.strangerID || skill != resurrectionScrollSkillID {
				t.Fatalf("MagicSkillUse = caster %d target %d skill %d, want %d on %d with %d", caster, target, skill, s.casterID, s.strangerID, resurrectionScrollSkillID)
			}
		})
	}
}
