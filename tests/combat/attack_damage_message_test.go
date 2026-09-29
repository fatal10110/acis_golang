package combat

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
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

// firstHitFeedback reads until the first damage-feedback run for a hit
// ends (a miss, a damage number, or a blocked notice) and returns the
// first Attack frame attackerID broadcast plus that run.
func firstHitFeedback(t *testing.T, c *scriptedClient, attackerID int32) (swingHit, []feedbackMessage) {
	t.Helper()
	var hit swingHit
	sawAttack := false
	var messages []feedbackMessage
	for end := c.Now().Add(5 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case serverpackets.OpcodeAttack:
			r := wireReader(frame[1:])
			if r.ReadInt32() != attackerID || sawAttack {
				continue
			}
			r.ReadInt32() // target id
			hit = swingHit{Damage: r.ReadInt32(), Flags: r.ReadUint8()}
			sawAttack = true
		case serverpackets.OpcodeSystemMessage:
			r := wireReader(frame[1:])
			m := feedbackMessage{ID: r.ReadInt32()}
			if !slices.Contains(damageFeedbackIDs, m.ID) {
				continue
			}
			if r.ReadInt32() == 1 {
				if typ := r.ReadInt32(); typ != serverpackets.SystemMessageParamNumber {
					t.Fatalf("message %d parameter type = %d, want a number", m.ID, typ)
				}
				m.Amount = r.ReadInt32()
			}
			messages = append(messages, m)
			switch m.ID {
			case serverpackets.SystemMessageCriticalHit, serverpackets.SystemMessageCriticalHitMagic,
				serverpackets.SystemMessageCriticalHitByPet, serverpackets.SystemMessageCriticalHitBySummonedMob:
			default:
				if !sawAttack {
					t.Fatalf("damage feedback %+v before the attacker's Attack frame", messages)
				}
				return hit, messages
			}
		}
	}
	t.Fatalf("no damage feedback run ended within 5s; got %+v", messages)
	return hit, nil
}

// playerSwingsAtHostile boots a level-5 player next to the fixture monster,
// installs roll as the player's combat random source, and starts an
// auto-attack on the monster.
func playerSwingsAtHostile(t *testing.T, roll func(int) int, prepare func(*hostileHandle)) (*scriptedClient, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	if prepare != nil {
		prepare(hostile)
	}
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	done := make(chan struct{})
	if !srv.PlayerQueue(t, objID).Post(func() {
		defer close(done)
		obj.(interface{ SetRollSource(func(int) int) }).SetRollSource(roll)
	}) {
		t.Fatal("player queue closed")
	}
	<-done

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	return c, objID
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

// TestPlayerAutoAttackDamageFeedback pins Player.sendDamageMessage on an
// auto-attack hit (CreatureAttack.java:234, Player.java:6610-6637): a miss
// reports MISSED_TARGET alone; a critical reports CRITICAL_HIT before the
// damage; a plain hit reports only YOU_DID_S1_DMG carrying the hit's
// damage; an invulnerable target turns the damage into ATTACK_WAS_BLOCKED,
// or OPPONENT_PETRIFIED when it is also paralyzed.
func TestPlayerAutoAttackDamageFeedback(t *testing.T) {
	t.Parallel()
	always := func(v int) func(int) int { return func(int) int { return v } }
	tests := []struct {
		name    string
		roll    func() func(int) int
		prepare func(*hostileHandle)
		want    func(swingHit) []feedbackMessage
	}{
		{
			name: "miss",
			roll: func() func(int) int { return always(999) },
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&attack.HitMiss == 0 {
					t.Fatalf("Attack flags = %#x, want a miss", h.Flags)
				}
				return []feedbackMessage{{ID: serverpackets.SystemMessageMissedTarget}}
			},
		},
		{
			name: "critical",
			roll: func() func(int) int { return always(0) },
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&attack.HitCritical == 0 {
					t.Fatalf("Attack flags = %#x, want a critical", h.Flags)
				}
				return []feedbackMessage{
					{ID: serverpackets.SystemMessageCriticalHit},
					{ID: serverpackets.SystemMessageYouDidS1Dmg, Amount: h.Damage},
				}
			},
		},
		{
			name: "plain hit",
			roll: landNoCrit,
			want: func(h swingHit) []feedbackMessage {
				if h.Flags&(attack.HitMiss|attack.HitCritical) != 0 {
					t.Fatalf("Attack flags = %#x, want a plain landed hit", h.Flags)
				}
				return []feedbackMessage{{ID: serverpackets.SystemMessageYouDidS1Dmg, Amount: h.Damage}}
			},
		},
		{
			name:    "invulnerable target",
			roll:    landNoCrit,
			prepare: func(h *hostileHandle) { h.SetInvul(true) },
			want: func(swingHit) []feedbackMessage {
				return []feedbackMessage{{ID: serverpackets.SystemMessageAttackWasBlocked}}
			},
		},
		{
			name: "petrified target",
			roll: func() func(int) int { return always(0) },
			prepare: func(h *hostileHandle) {
				h.SetInvul(true)
				h.SetParalyzed(true)
			},
			want: func(swingHit) []feedbackMessage {
				return []feedbackMessage{
					{ID: serverpackets.SystemMessageCriticalHit},
					{ID: serverpackets.SystemMessageOpponentPetrified},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, objID := playerSwingsAtHostile(t, tt.roll(), tt.prepare)
			hit, got := firstHitFeedback(t, c, objID)
			if want := tt.want(hit); !slices.Equal(got, want) {
				t.Fatalf("damage feedback = %+v, want %+v", got, want)
			}
		})
	}
}

// TestNPCAutoAttackSendsNoDamageFeedback pins the base
// Creature.sendDamageMessage no-op: an NPC's hit on a player reports none
// of the attacker-side damage messages to anyone.
func TestNPCAutoAttackSendsNoDamageFeedback(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)
	victim := livePlayer(t, srv, objID)
	attacker := srv.SpawnAttackingHostileNPCAt(t, location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	attacker.DoAttack(t, victim.(attackable.Combatant))
	assertAttackBy(t, c, attacker.ObjectID())
	for end := c.Now().Add(2 * time.Second); c.Now().Before(end); {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil || frame[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id := wireReader(frame[1:]).ReadInt32(); slices.Contains(damageFeedbackIDs, id) {
			t.Fatalf("NPC hit sent damage feedback message %d", id)
		}
	}
}
