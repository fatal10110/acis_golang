package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/cubic"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLifeCubicHealsWoundedPartyMember grants a full-HP owner in a party a
// Life Cubic and lets it heal a wounded member. Cubic.pickFriendlyTarget
// (Cubic.java:209-252) scans the owner's party for the lowest HP ratio
// under full, so the member is healed rather than the owner;
// Cubic.useHealSkill (Cubic.java:364-373) then sends the member its own
// StatusUpdate before REJUVENATING_HP, and the owner neither.
func TestLifeCubicHealsWoundedPartyMember(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Owner", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: summonLifeCubicSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				SkillType: "SUMMON", IsCubic: true, NpcID: int(cubic.Life),
				CubicActivationTime: lifeCubicInterval, SummonTotalLifeTime: 900_000,
				StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
			},
			{ID: lifeCubicHealSkill, Level: 1, Power: lifeCubicHealPower},
		})),
	)
	c, ownerID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, ownerID, summonLifeCubicSkill, 1)
	startInWorld(t, c)
	memberID := srv.SeedCharacterFor(t, "player2", "Member", 20, 0).ID
	member := srv.DialClient(t, "player2", 1)
	startInWorldAmongPlayers(t, member)
	drainUntilQuiet(t, c)

	c.Send(encodePartyInvite("Member"))
	drainUntilQuiet(t, c)
	member.Send(encodePartyAnswer(1))
	drainUntilQuiet(t, member)
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(ownerID)
	if !ok {
		t.Fatalf("world player %d missing", ownerID)
	}
	owner, ok := obj.(interface{ SetRollSource(func(int) int) })
	if !ok {
		t.Fatalf("world player %d = %T, want SetRollSource", ownerID, obj)
	}
	owner.SetRollSource(func(int) int { return 0 })

	before, _ := damageToHealHeadroom(t, srv, memberID, 2*lifeCubicHealPower)
	ownerHP := srv.PlayerMaxHP(t, ownerID)
	srv.AddPlayerHP(t, ownerID, float64(ownerHP))
	if hp := srv.PlayerCurrentHP(t, ownerID); hp != ownerHP {
		t.Fatalf("owner HP = %d, want full %d", hp, ownerHP)
	}
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, member)

	c.Send(encodeRequestMagicSkillUse(summonLifeCubicSkill, false, false))
	srv.Advance(t, time.Second)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, member)

	want := before + lifeCubicHealPower
	srv.AdvanceUntil(t, "the Life Cubic healing the member", func() bool { return srv.PlayerCurrentHP(t, memberID) == want })

	statuses, _ := selfStatusesThenMessage(t, member, memberID, serverpackets.SystemMessageRejuvenatingHP, 1)
	assertCasterStatus(t, srv, statuses[0], memberID, want, srv.PlayerCurrentMP(t, memberID))

	for _, frame := range collectQuiet(t, c) {
		switch frame[0] {
		case serverpackets.OpcodeSystemMessage:
			if wire.NewReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageRejuvenatingHP {
				t.Fatal("the owner was told of a heal that landed on its party member")
			}
		case serverpackets.OpcodeStatusUpdate:
			if wire.NewReader(frame[1:]).ReadInt32() == ownerID {
				t.Fatal("the owner got its own status for a heal that landed on its party member")
			}
		}
	}
	if hp := srv.PlayerCurrentHP(t, ownerID); hp != ownerHP {
		t.Fatalf("owner HP = %d, want untouched %d", hp, ownerHP)
	}
}

func collectQuiet(t *testing.T, c interface{ ReadWithTimeout(time.Duration) []byte }) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return frames
		}
		frames = append(frames, frame)
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}

func encodePartyInvite(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodePartyAnswer(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestAnswerJoinParty)
	w.WriteInt32(response)
	return w.Bytes()
}
