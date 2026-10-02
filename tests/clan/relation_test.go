package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// pupilID is a Rivals academy member seeded for the relation tests.
const pupilID int32 = 0x7f200101

// The RelationChanged bits a clan relation sets (RelationChanged.java).
const (
	relLeader    = serverpackets.RelationLeader
	relOneSided  = serverpackets.RelationOneSidedWar
	relMutualWar = serverpackets.RelationMutualWar
)

// relationView is one RelationChanged a player was sent.
type relationView struct {
	objectID, relation, autoAttackable, karma, pvpFlag int32
}

// relationAfterCharInfo returns the RelationChanged that follows objectID's
// CharInfo in frames, failing when that CharInfo is missing or not
// followed by one for the same player.
func relationAfterCharInfo(t *testing.T, frames [][]byte, objectID int32) relationView {
	t.Helper()
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(f[1:])
		for range 4 {
			r.ReadInt32()
		}
		if r.ReadInt32() != objectID {
			continue
		}
		if i+1 >= len(frames) || frames[i+1][0] != serverpackets.OpcodeRelationChanged {
			t.Fatalf("CharInfo of %d not followed by RelationChanged: %x", objectID, opcodes(frames[i:]))
		}
		r = wire.NewReader(frames[i+1][1:])
		v := relationView{r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()}
		if v.objectID != objectID {
			t.Fatalf("RelationChanged after CharInfo of %d describes %d", objectID, v.objectID)
		}
		return v
	}
	t.Fatalf("no CharInfo of %d in %x", objectID, opcodes(frames))
	return relationView{}
}

// bootRelationWorld boots the founder leading Knights and the rival leading
// Rivals, plus the academy member Pupil in Rivals, with stmts seeding their
// wars. Only the founder is in the world.
func bootRelationWorld(t *testing.T, stmts ...string) *gameservertest.Server {
	t.Helper()
	stmts = append([]string{
		`UPDATE characters SET clanid = ` + itoa(rivalsClanID) + `, subpledge = -1, power_grade = 9, lvl_joined_academy = 10 WHERE obj_Id = ` + itoa(pupilID),
	}, stmts...)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Founder", 10, 0),
		gameservertest.WithWantChars(1),
		seedRival(t, castMember{pupilID, "player3", "Pupil"}),
		seedAllianceClans(t, stmts...),
	)
	startInWorld(t, srv.Client)
	drainFrames(t, srv.Client)
	return srv
}

// enterBeside brings account's character into the world beside the
// founder, returning its client and the frames it and the founder read.
func enterBeside(t *testing.T, srv *gameservertest.Server, account string) (c *testsupport.ScriptedClient, own, founder [][]byte) {
	t.Helper()
	c = srv.DialClient(t, account, 1)
	own = startInWorld(t, c)
	return c, own, drainFrames(t, srv.Client)
}

func warStmt(from, to int32) string {
	return `INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES ('` + itoa(from) + `', '` + itoa(to) + `', 0)`
}

// TestDiscoveryShowsClanWarRelations has two clan leaders meet. Each is
// sent the other's relation right after its CharInfo (Player.sendInfo):
// the leader bit, plus the one-sided war bit when the viewer's clan
// declared war on the other's, and the mutual war bit when that clan
// declared back (Player.getRelation, Player.java:805-836).
func TestDiscoveryShowsClanWarRelations(t *testing.T) {
	t.Parallel()
	mutual := []string{warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID)}
	oneSided := []string{warStmt(rivalsClanID, knightsClanID)}
	for _, tt := range []struct {
		name             string
		stmts            []string
		founderSeesRival int32
		rivalSeesFounder int32
	}{
		{"no war", nil, relLeader, relLeader},
		{"mutual war", mutual, relLeader | relOneSided | relMutualWar, relLeader | relOneSided | relMutualWar},
		{"rivals declared only", oneSided, relLeader, relLeader | relOneSided},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootRelationWorld(t, tt.stmts...)
			founderID := srv.SoleObjectID(t)
			_, own, founder := enterBeside(t, srv, "player2")
			if got := relationAfterCharInfo(t, founder, rivalLeaderID); got != (relationView{rivalLeaderID, tt.founderSeesRival, 0, 0, 0}) {
				t.Fatalf("founder's view of the rival = %+v, want relation %#x", got, tt.founderSeesRival)
			}
			if got := relationAfterCharInfo(t, own, founderID); got != (relationView{founderID, tt.rivalSeesFounder, 0, 0, 0}) {
				t.Fatalf("rival's view of the founder = %+v, want relation %#x", got, tt.rivalSeesFounder)
			}
		})
	}
}

// TestAcademyMemberHidesClanWar has a Rivals academy member meet the
// Knights leader while the clans are at war: an academy member on either
// side drops the war bits (Player.java:826-833), so each sees only what
// remains — the leader bit of the Knights leader, nothing of the pupil.
func TestAcademyMemberHidesClanWar(t *testing.T) {
	t.Parallel()
	srv := bootRelationWorld(t, warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID))
	founderID := srv.SoleObjectID(t)
	_, own, founder := enterBeside(t, srv, "player3")
	if got := relationAfterCharInfo(t, founder, pupilID); got.relation != 0 {
		t.Fatalf("founder's view of the pupil = %+v, want no relation", got)
	}
	if got := relationAfterCharInfo(t, own, founderID); got.relation != relLeader {
		t.Fatalf("pupil's view of the founder = %+v, want the leader bit alone", got)
	}
}

func encodeChangeMoveType(run bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeMoveType)
	w.WriteInt32(wire.BoolInt32(run))
	return w.Bytes()
}

// TestCharInfoRefreshCarriesRelation has the founder, at mutual war with
// the rival's clan, change stance: the rival's CharInfo refresh is followed
// by the founder's relation to it (Player.broadcastCharInfo,
// Player.java:2254-2266).
func TestCharInfoRefreshCarriesRelation(t *testing.T) {
	t.Parallel()
	srv := bootRelationWorld(t, warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID))
	founderID := srv.SoleObjectID(t)
	rival, _, _ := enterBeside(t, srv, "player2")
	drainFrames(t, rival)

	srv.Client.Send(encodeChangeMoveType(false))
	frames := drainFrames(t, rival)
	if got := relationAfterCharInfo(t, frames, founderID); got != (relationView{founderID, relLeader | relOneSided | relMutualWar, 0, 0, 0}) {
		t.Fatalf("rival's refreshed view of the founder = %+v", got)
	}
}
