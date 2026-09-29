package pets

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// feedbackMessage is one attacker damage-feedback system message; Amount is
// its number parameter, if it has one.
type feedbackMessage struct {
	ID     int32
	Amount int32
}

// swingHit is the first hit of an Attack frame.
type swingHit struct {
	Damage int32
	Flags  uint8
}

// damageFeedbackIDs is every message a hit can report to its attacking side.
var damageFeedbackIDs = []int32{
	serverpackets.SystemMessageMissedTarget, serverpackets.SystemMessageCriticalHit,
	serverpackets.SystemMessageCriticalHitMagic, serverpackets.SystemMessageYouDidS1Dmg,
	serverpackets.SystemMessagePetHitForS1Damage, serverpackets.SystemMessageCriticalHitByPet,
	serverpackets.SystemMessageSummonGaveDamageS1, serverpackets.SystemMessageCriticalHitBySummonedMob,
	serverpackets.SystemMessageOpponentPetrified, serverpackets.SystemMessageAttackWasBlocked,
}

// summonSwingFeedback reads for window after attackerID's first Attack frame
// arrives and returns that frame's first hit plus every damage-feedback
// message read, until the first run for a landed hit ends.
func summonSwingFeedback(t *testing.T, c *testsupport.ScriptedClient, attackerID int32, window time.Duration) (swingHit, []feedbackMessage) {
	t.Helper()
	var hit swingHit
	sawAttack := false
	var messages []feedbackMessage
	end := c.Now().Add(5 * time.Second)
	for c.Now().Before(end) {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			r := wire.NewReader(frame[1:])
			if r.ReadInt32() != attackerID || sawAttack {
				continue
			}
			r.ReadInt32() // target id
			hit = swingHit{Damage: r.ReadInt32(), Flags: r.ReadUint8()}
			sawAttack = true
			end = c.Now().Add(window)
		case serverpackets.OpcodeSystemMessage:
			r := wire.NewReader(frame[1:])
			m := feedbackMessage{ID: r.ReadInt32()}
			if !slices.Contains(damageFeedbackIDs, m.ID) {
				continue
			}
			if r.ReadInt32() == 1 {
				r.ReadInt32() // parameter type
				m.Amount = r.ReadInt32()
			}
			messages = append(messages, m)
			if m.ID == serverpackets.SystemMessagePetHitForS1Damage || m.ID == serverpackets.SystemMessageSummonGaveDamageS1 {
				return hit, messages
			}
		}
	}
	if !sawAttack {
		t.Fatalf("summon %d never swung", attackerID)
	}
	return hit, messages
}

// setSummonRoll installs roll as s's combat random source on its owner's
// queue, where the summon's attacks run.
func setSummonRoll(t *testing.T, srv *gameservertest.Server, ownerID int32, s *summon.Actor, roll func(int) int) {
	t.Helper()
	done := make(chan struct{})
	if !srv.PlayerQueue(t, ownerID).Post(func() {
		defer close(done)
		s.SetRollSource(roll)
	}) {
		t.Fatal("owner queue closed")
	}
	<-done
}

// landNoCrit makes every hit roll succeed and every critical roll fail: the
// hit and critical rolls alternate within a swing.
func landNoCrit() func(int) int {
	calls := 0
	return func(int) int {
		calls++
		if calls%2 == 1 {
			return 0
		}
		return 999
	}
}

// TestPetAutoAttackDamageFeedbackReachesOwner pins Summon.sendDamageMessage
// on a pet's auto-attack (Summon.java:269-293): the owner reads
// CRITICAL_HIT_BY_PET before PET_HIT_FOR_S1_DAMAGE carrying the hit's
// damage on a critical, the damage alone on a plain hit, and nothing on a
// miss.
func TestPetAutoAttackDamageFeedbackReachesOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		roll func() func(int) int
		want func(swingHit) []feedbackMessage
	}{
		{
			name: "critical",
			roll: func() func(int) int { return func(int) int { return 0 } },
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&attack.HitCritical == 0 {
					t.Fatalf("pet Attack flags = %#x, want a critical", h.Flags)
				}
				return []feedbackMessage{
					{ID: serverpackets.SystemMessageCriticalHitByPet},
					{ID: serverpackets.SystemMessagePetHitForS1Damage, Amount: h.Damage},
				}
			},
		},
		{
			name: "plain hit",
			roll: landNoCrit,
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&(attack.HitMiss|attack.HitCritical) != 0 {
					t.Fatalf("pet Attack flags = %#x, want a plain landed hit", h.Flags)
				}
				return []feedbackMessage{{ID: serverpackets.SystemMessagePetHitForS1Damage, Amount: h.Damage}}
			},
		},
		{
			name: "miss",
			roll: func() func(int) int { return func(int) int { return 999 } },
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&attack.HitMiss == 0 {
					t.Fatalf("pet Attack flags = %#x, want a miss", h.Flags)
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// A template critical rate lets a zero roll land a critical.
			wolf := wolfTemplate()
			wolf.CritRate = 4
			h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
				gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolf, treeTemplate()})),
			})
			petActor, _ := h.spawnWolf(t)
			hostile := h.srv.SpawnHostileNPC(t)
			drainUntilQuiet(t, h.client)
			setSummonRoll(t, h.srv, h.ownerID, petActor, tt.roll())

			h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
			drainFrames(t, h.client)
			h.client.Send(encodeRequestActionUse(petAttackAction, false))

			hit, got := summonSwingFeedback(t, h.client, petActor.ObjectID(), 2*time.Second)
			if want := tt.want(hit); !slices.Equal(got, want) {
				t.Fatalf("owner damage feedback = %+v, want %+v", got, want)
			}
		})
	}
}

// TestServitorAutoAttackDamageFeedbackReachesOwner pins
// Servitor.sendDamageMessage (Servitor.java:82-106): the owner reads
// SUMMON_GAVE_DAMAGE_S1 carrying the servitor's hit damage, never the pet
// or player family.
func TestServitorAutoAttackDamageFeedbackReachesOwner(t *testing.T) {
	t.Parallel()
	const (
		summonCatSkill = 1111
		catNPCID       = 12600
	)
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: summonCatSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: catNPCID, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: catNPCID, TemplateID: catNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, PAtk: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		BaseAttackRange: 40, CollisionRadius: 8, CollisionHeight: 20,
	}
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
		gameservertest.WithSkills(skills))
	c, ownerID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, summonCatSkill, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, c)
	c.Send(encodeRequestMagicSkillUse(summonCatSkill))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)
	setSummonRoll(t, srv, ownerID, servitor, landNoCrit())

	c.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, c)
	c.Send(encodeRequestActionUse(petAttackAction, false))

	hit, got := summonSwingFeedback(t, c, servitor.ObjectID(), 2*time.Second)
	if hit.Flags&(attack.HitMiss|attack.HitCritical) != 0 {
		t.Fatalf("servitor Attack flags = %#x, want a plain landed hit", hit.Flags)
	}
	if want := []feedbackMessage{{ID: serverpackets.SystemMessageSummonGaveDamageS1, Amount: hit.Damage}}; !slices.Equal(got, want) {
		t.Fatalf("owner damage feedback = %+v, want %+v", got, want)
	}
}
