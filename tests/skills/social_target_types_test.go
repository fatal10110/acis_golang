package skills

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	socialServitorSkillID   int32 = 1111
	socialServitorNpcID     int32 = 12077
	socialCorpseAllySkillID int32 = 1254
)

// socialServitorSkill summons a servitor at its caster.
func socialServitorSkill() modelskill.Definition {
	return modelskill.Definition{
		ID: modelskill.ID(socialServitorSkillID), Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: int(socialServitorNpcID), SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, StaticReuse: true,
	}
}

// socialServitorNPCs is the servitor template socialServitorSkill spawns.
func socialServitorNPCs() gameservertest.Option {
	return gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
		ID: int(socialServitorNpcID), TemplateID: int(socialServitorNpcID), Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}}))
}

// summonMateServitor has Mate summon its servitor and returns its id.
func (s *socialScene) summonMateServitor(t *testing.T) int32 {
	t.Helper()
	s.mate.Send(encodeRequestMagicSkillUse(socialServitorSkillID, false, false))
	var id int32
	s.srv.AdvanceUntil(t, "Mate's servitor in world state", func() bool {
		obj, ok := s.srv.State.Summon(s.mateID)
		if ok {
			id = obj.ObjectID()
		}
		return ok
	})
	s.quiet(t)
	return id
}

// TestPartyMemberSkillReachesPartyMembers casts a PARTY_MEMBER buff:
// TargetPartyMember.meetCastConditions accepts any living playable whose
// acting player is in the caster's party (TargetPartyMember.java:44-75), so
// the cast starts on Mate and on Mate's servitor once they share a party,
// and the same servitor is refused while they do not.
func TestPartyMemberSkillReachesPartyMembers(t *testing.T) {
	t.Parallel()
	def := targetedSkill(partyMemberSkillID, modelskill.TargetPartyMember, false)
	for _, tt := range []struct {
		name     string
		summon   bool
		party    bool
		accepted bool
	}{
		{"party member", false, true, true},
		{"party member's servitor", true, true, true},
		{"servitor outside the party", true, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := bootSocialSceneKnowing(t, def, []modelskill.Definition{socialServitorSkill()}, socialServitorNPCs())
			if tt.party {
				s.formParty(t)
			}
			target := s.mateID
			if tt.summon {
				target = s.summonMateServitor(t)
			}
			x, y, z := s.srv.PlayerPosition(t, s.mateID)
			s.caster.Send(encodeAction(target, int32(x), int32(y), int32(z), false))
			s.quiet(t)
			s.caster.Send(encodeRequestMagicSkillUse(partyMemberSkillID, false, false))
			if tt.accepted {
				readCastStartFrames(t, s.caster, s.casterID, partyMemberSkillID, 1, 500, 60_000, target)
				return
			}
			assertSystemMessageSkillFrame(t, s.caster.Read(), serverpackets.SystemMessageS1CannotBeUsed, partyMemberSkillID, 1)
			assertNoActionFailedUntilQuiet(t, s.caster, "servitor outside the party")
		})
	}
}

// TestCorpseAllySkillListsDeadClanAndAlliance kills Mate and Stranger and
// casts a CORPSE_ALLY skill: TargetCorpseAlly lists every dead player in
// radius sharing the caster's clan or alliance, or the caster alone when
// there is none (TargetCorpseAlly.java:24-52). The dead clanmate is always
// listed, the dead player of another clan only once both clans are allied,
// and a caster without a clan lists only itself.
func TestCorpseAllySkillListsDeadClanAndAlliance(t *testing.T) {
	t.Parallel()
	def := targetedSkill(socialCorpseAllySkillID, modelskill.TargetCorpseAlly, false)
	for _, tt := range []struct {
		name   string
		clans  bool
		allied bool
	}{{"clans apart", true, false}, {"clans allied", true, true}, {"no clan", false, false}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var opts []gameservertest.Option
			if tt.clans {
				opts = append(opts, socialClans(t, tt.allied))
			}
			s := bootSocialScene(t, def, opts...)
			dieOnQueue(t, s.srv, s.mateID)
			dieOnQueue(t, s.srv, s.strangerID)
			s.quiet(t)
			var want []int32
			switch {
			case tt.allied:
				want = []int32{s.mateID, s.strangerID}
			case tt.clans:
				want = []int32{s.mateID}
			default:
				want = []int32{s.casterID}
			}
			got := launchedTargets(t, s, def)
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("CORPSE_ALLY targets = %v, want %v", got, want)
			}
		})
	}
}
