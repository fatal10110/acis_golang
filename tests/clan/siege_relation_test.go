package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The RelationChanged siege bits (RelationChanged.java).
const (
	relInSiege  = serverpackets.RelationInSiege
	relAttacker = serverpackets.RelationAttacker
	relAlly     = serverpackets.RelationAlly
	relEnemy    = serverpackets.RelationEnemy
)

// lastRelation returns the relation bits of the last RelationChanged in
// frames describing objectID.
func lastRelation(t *testing.T, frames [][]byte, objectID int32) int32 {
	t.Helper()
	relation, found := int32(0), false
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeRelationChanged {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() == objectID {
			relation, found = r.ReadInt32(), true
		}
	}
	if !found {
		t.Fatalf("no RelationChanged of %d in %x", objectID, opcodes(frames))
	}
	return relation
}

// TestSiegeRelations has Knights attack Gludio and Rivals defend it. A
// player in a siege shows the in-siege bit, the attacker bit when it
// attacks, and the enemy bit to a viewer of another siege state or the
// ally bit to one of its own (Player.getRelation, Player.java:819-828).
// The siege start sends each its new relation (Siege.updatePlayerSiegeStateFlags),
// and a Knights member entering the world during the siege shows and sees
// both sides so.
func TestSiegeRelations(t *testing.T) {
	t.Parallel()
	w := bootSiegeClanWorld(t, []castMember{third}, []string{
		registerOnGludio(knightsClanID, "ATTACKER"),
		registerOnGludio(rivalsClanID, "DEFENDER"),
		`UPDATE characters SET clanid = ` + itoa(knightsClanID) + `, power_grade = 6 WHERE obj_Id = ` + itoa(thirdID),
	})
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	gludio.Start()
	founder, rival := drainFrames(t, w.leader), drainFrames(t, w.member)
	if got, want := lastRelation(t, founder, w.memberID), int32(relLeader|relInSiege|relEnemy); got != want {
		t.Fatalf("founder's view of the defending rival = %#x, want %#x", got, want)
	}
	if got, want := lastRelation(t, rival, w.leaderID), int32(relLeader|relInSiege|relEnemy|relAttacker); got != want {
		t.Fatalf("rival's view of the attacking founder = %#x, want %#x", got, want)
	}

	c := w.srv.DialClient(t, third.account, 1)
	own := startInWorld(t, c)
	founder, rival = drainFrames(t, w.leader), drainFrames(t, w.member)
	for _, tt := range []struct {
		name    string
		frames  [][]byte
		subject int32
		want    int32
	}{
		{"third's view of the founder", own, w.leaderID, relLeader | relInSiege | relAlly | relAttacker},
		{"third's view of the rival", own, w.memberID, relLeader | relInSiege | relEnemy},
		{"founder's view of third", founder, thirdID, relInSiege | relAlly | relAttacker},
		{"rival's view of third", rival, thirdID, relInSiege | relEnemy | relAttacker},
	} {
		if got := relationAfterCharInfo(t, tt.frames, tt.subject).relation; got != tt.want {
			t.Fatalf("%s = %#x, want %#x", tt.name, got, tt.want)
		}
	}
}

// TestSiegeKillCounts has the Rivals leader kill the Knights founder, both
// in a PvP zone. A kill there awards neither karma nor a PvP point; when
// both players fight the siege on opposite sides it counts a siege kill
// for the killer's clan and a siege death for the victim's
// (Player.onKillUpdatePvPKarma, Player.java:2784-2800).
func TestSiegeKillCounts(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name            string
		rivals, knights string
		counted         bool
	}{
		{"opposite sides", "DEFENDER", "ATTACKER", true},
		{"same side", "ATTACKER", "ATTACKER", false},
		{"victim outside the siege", "ATTACKER", "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stmts := []string{registerOnGludio(rivalsClanID, tt.rivals)}
			if tt.knights != "" {
				stmts = append(stmts, registerOnGludio(knightsClanID, tt.knights))
			}
			w := bootSiegeClanWorld(t, nil, stmts)
			startGludio(t, w)
			setInPvPZone(t, w, w.leaderID, true)
			setInPvPZone(t, w, w.memberID, true)

			killPlayer(t, w, w.leaderID, w.memberID)
			want := 0
			if tt.counted {
				want = 1
			}
			if kills, deaths := w.srv.Sieges.Kills(rivalsClanID), w.srv.Sieges.Deaths(knightsClanID); kills != want || deaths != want {
				t.Fatalf("siege kills of Rivals, deaths of Knights = %d, %d, want %d each", kills, deaths, want)
			}
			if kills, deaths := w.srv.Sieges.Kills(knightsClanID), w.srv.Sieges.Deaths(rivalsClanID); kills != 0 || deaths != 0 {
				t.Fatalf("siege kills of Knights, deaths of Rivals = %d, %d, want none", kills, deaths)
			}
			w.leaveWorld(t, w.member)
			if got := storedKillStanding(t, w, w.memberID); got != (killStanding{}) {
				t.Fatalf("killer's stored standing = %+v, want none", got)
			}
		})
	}
}
