package clan

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The founder's experience after a death at level 10, whose span is
// 48229-71201 (22972) with 8.875% lost: a full loss is 2039, a quarter loss
// (2.21875%) 510.
const (
	foundExp         = 60000
	fullDeathLoss    = 2039
	quarterDeathLoss = 510
)

// onlineCharacter returns the character of the player id in the world.
func onlineCharacter(t *testing.T, srv *gameservertest.Server, id int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(id)
	if !ok {
		t.Fatalf("player %d not in world", id)
	}
	c, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return c
}

// onPlayerQueue runs fn on the queue of the player id and waits for it.
func onPlayerQueue(t *testing.T, srv *gameservertest.Server, id int32, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !srv.PlayerQueue(t, id).Post(func() { fn(); close(done) }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// killPlayer has the player killerID kill the player victimID.
func killPlayer(t *testing.T, w *clanWorld, victimID, killerID int32) {
	t.Helper()
	obj, ok := w.srv.State.Player(killerID)
	if !ok {
		t.Fatalf("killer %d not in world", killerID)
	}
	killer, ok := obj.(attackable.Combatant)
	if !ok {
		t.Fatalf("killer %T is not a combatant", obj)
	}
	victim := onlineCharacter(t, w.srv, victimID)
	onPlayerQueue(t, w.srv, victimID, func() {
		if !victim.Kill(killer) {
			t.Errorf("Kill() = false on living player %d", victimID)
		}
	})
}

// headerReputation reads the reputation, the seventh field, of a
// PledgeShowInfoUpdate frame, and the clan it describes.
func headerReputation(t *testing.T, frame []byte) (clanID, reputation int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodePledgeShowInfoUpdate {
		t.Fatalf("opcode = %#x, want PledgeShowInfoUpdate", frame[0])
	}
	r := wire.NewReader(frame[1:])
	clanID = r.ReadInt32()
	for range 5 {
		r.ReadInt32()
	}
	return clanID, r.ReadInt32()
}

// TestClanWarDeath has the Knights founder killed by a Rivals member, each
// clan level 5. The death costs a quarter of the normal experience when
// either clan declared war on the other. When both did, the kill moves
// reputation: Rivals gains 1 while Knights holds a positive score, then
// Knights loses 1 while Rivals (its gain counted) holds one, and each
// clan's members get its new header; a score rising above 0 first turns
// the clan skills on, and one falling to 0 or below turns them off. An
// academy killer, an academy member's death (Pupil in Knights' academy,
// killed instead of the founder), or a death in an arena moves none; the
// arena death also costs no experience.
func TestClanWarDeath(t *testing.T) {
	t.Parallel()
	mutual := []string{warStmt(knightsClanID, rivalsClanID), warStmt(rivalsClanID, knightsClanID)}
	for _, tt := range []struct {
		name                     string
		wars                     []string
		knightsRep, rivalsRep    int
		byPupil, inArena         bool
		academyVictim            bool
		wantKnights, wantRivals  int
		wantLoss                 int64
		wantRivalsActivatedSkill bool
		wantKnightsDeactivated   bool
	}{
		{name: "no war", knightsRep: 100, rivalsRep: 100, wantKnights: 100, wantRivals: 100, wantLoss: fullDeathLoss},
		{name: "mutual war", wars: mutual, knightsRep: 100, rivalsRep: 100, wantKnights: 99, wantRivals: 101, wantLoss: quarterDeathLoss},
		{
			name: "killer's clan declared only", wars: []string{warStmt(rivalsClanID, knightsClanID)}, knightsRep: 100, rivalsRep: 100,
			wantKnights: 100, wantRivals: 100, wantLoss: quarterDeathLoss,
		},
		{
			name: "victim's clan declared only", wars: []string{warStmt(knightsClanID, rivalsClanID)}, knightsRep: 100, rivalsRep: 100,
			wantKnights: 100, wantRivals: 100, wantLoss: quarterDeathLoss,
		},
		{name: "victim's clan at 0", wars: mutual, knightsRep: 0, rivalsRep: 100, wantKnights: -1, wantRivals: 100, wantLoss: quarterDeathLoss},
		{
			name: "killer's clan at 0", wars: mutual, knightsRep: 100, rivalsRep: 0, wantKnights: 99, wantRivals: 1, wantLoss: quarterDeathLoss,
			wantRivalsActivatedSkill: true,
		},
		{
			name: "victim's clan crossing to 0", wars: mutual, knightsRep: 1, rivalsRep: 100, wantKnights: 0, wantRivals: 101, wantLoss: quarterDeathLoss,
			wantKnightsDeactivated: true,
		},
		{
			name: "academy victim", wars: mutual, knightsRep: 100, rivalsRep: 100, academyVictim: true,
			wantKnights: 100, wantRivals: 100, wantLoss: quarterDeathLoss,
		},
		{name: "academy killer", wars: mutual, knightsRep: 100, rivalsRep: 100, byPupil: true, wantKnights: 100, wantRivals: 100, wantLoss: quarterDeathLoss},
		{name: "arena", wars: mutual, knightsRep: 100, rivalsRep: 100, inArena: true, wantKnights: 100, wantRivals: 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			pupilClanID := rivalsClanID
			if tt.academyVictim {
				pupilClanID = knightsClanID
			}
			stmts := append([]string{
				`UPDATE clan_data SET reputation_score = ` + itoa(int32(tt.knightsRep)) + ` WHERE clan_id = ` + itoa(knightsClanID),
				`UPDATE clan_data SET reputation_score = ` + itoa(int32(tt.rivalsRep)) + ` WHERE clan_id = ` + itoa(rivalsClanID),
				`UPDATE characters SET clanid = ` + itoa(pupilClanID) + `, subpledge = -1, power_grade = 9, lvl_joined_academy = 10, exp = ` +
					itoa(foundExp) + ` WHERE obj_Id = ` + itoa(pupilID),
			}, tt.wars...)
			w := bootAllianceCast(t, []castMember{{pupilID, "player3", "Pupil"}}, stmts, gameservertest.WithAllowDelevel(true))
			killerID, victimID, victimClient := w.memberID, w.leaderID, w.leader
			if tt.byPupil || tt.academyVictim {
				pupil := w.srv.DialClient(t, "player3", 1)
				startInWorld(t, pupil)
				if tt.byPupil {
					killerID = pupilID
				} else {
					victimID, victimClient = pupilID, pupil
				}
				drainFrames(t, w.leader)
				drainFrames(t, w.member)
				drainFrames(t, pupil)
			}
			if tt.inArena {
				founder := onlineCharacter(t, w.srv, w.leaderID)
				onPlayerQueue(t, w.srv, w.leaderID, func() { founder.SetInPvPZone(true) })
			}

			killPlayer(t, w, victimID, killerID)
			founder, rival := drainFrames(t, w.leader), drainFrames(t, w.member)
			if tt.academyVictim {
				if _, ok := firstOpcode(drainFrames(t, victimClient), serverpackets.OpcodePledgeShowInfoUpdate); ok {
					t.Fatal("academy victim got a clan header, want none")
				}
			}

			moved := tt.wantKnights != tt.knightsRep
			header, ok := firstOpcode(founder, serverpackets.OpcodePledgeShowInfoUpdate)
			if ok != moved {
				t.Fatalf("founder's header sent = %v, want %v", ok, moved)
			}
			if ok {
				if id, rep := headerReputation(t, header); id != knightsClanID || rep != int32(tt.wantKnights) {
					t.Fatalf("founder's header = clan %d reputation %d, want %d %d", id, rep, knightsClanID, tt.wantKnights)
				}
			}
			moved = tt.wantRivals != tt.rivalsRep
			header, ok = firstOpcode(rival, serverpackets.OpcodePledgeShowInfoUpdate)
			if ok != moved {
				t.Fatalf("rival's header sent = %v, want %v", ok, moved)
			}
			if ok {
				if id, rep := headerReputation(t, header); id != rivalsClanID || rep != int32(tt.wantRivals) {
					t.Fatalf("rival's header = clan %d reputation %d, want %d %d", id, rep, rivalsClanID, tt.wantRivals)
				}
			}
			activated := slices.Contains(messages(t, rival), serverpackets.SystemMessageClanSkillsActivatedReputation)
			if activated != tt.wantRivalsActivatedSkill {
				t.Fatalf("rival told the clan skills turned on = %v, want %v", activated, tt.wantRivalsActivatedSkill)
			}
			if tt.wantRivalsActivatedSkill {
				notice := slices.IndexFunc(rival, func(f []byte) bool {
					if f[0] != serverpackets.OpcodeSystemMessage {
						return false
					}
					id, _ := sysMsg(t, f)
					return id == serverpackets.SystemMessageClanSkillsActivatedReputation
				})
				if got := only(rival[notice+1:], serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowInfoUpdate); string(got) !=
					string([]byte{serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowInfoUpdate}) {
					t.Fatalf("rival's crossing after the notice = %x, want SkillList, then the header", got)
				}
			}

			deactivated := slices.Contains(messages(t, founder), serverpackets.SystemMessageReputationLowClanSkillsDeactivated)
			if deactivated != tt.wantKnightsDeactivated {
				t.Fatalf("founder told the clan skills turned off = %v, want %v", deactivated, tt.wantKnightsDeactivated)
			}
			if tt.wantKnightsDeactivated {
				notice := slices.IndexFunc(founder, func(f []byte) bool {
					if f[0] != serverpackets.OpcodeSystemMessage {
						return false
					}
					id, _ := sysMsg(t, f)
					return id == serverpackets.SystemMessageReputationLowClanSkillsDeactivated
				})
				if got := only(founder[notice+1:], serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowInfoUpdate); string(got) !=
					string([]byte{serverpackets.OpcodeSkillList, serverpackets.OpcodePledgeShowInfoUpdate}) {
					t.Fatalf("founder's crossing after the notice = %x, want SkillList, then the header", got)
				}
				// The death runs on the killer's queue; the clan skills the
				// founder loses come off on its own, so the crossing reaches
				// it behind the death's experience-loss UserInfo.
				isUserInfo := func(f []byte) bool { return f[0] == serverpackets.OpcodeUserInfo }
				if !slices.ContainsFunc(founder[:notice], isUserInfo) || slices.ContainsFunc(founder[notice:], isUserInfo) {
					t.Fatalf("founder's frames %x: want the crossing notice after every UserInfo of the death", opcodes(founder))
				}
			}

			w.leaveWorld(t, victimClient)
			if got := queryInt(t, w, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, knightsClanID); got != int64(tt.wantKnights) {
				t.Fatalf("stored Knights reputation = %d, want %d", got, tt.wantKnights)
			}
			if got := queryInt(t, w, `SELECT reputation_score FROM clan_data WHERE clan_id = ?`, rivalsClanID); got != int64(tt.wantRivals) {
				t.Fatalf("stored Rivals reputation = %d, want %d", got, tt.wantRivals)
			}
			if got := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, victimID); got != foundExp-tt.wantLoss {
				t.Fatalf("victim's exp after the death = %d, want %d", got, foundExp-tt.wantLoss)
			}
		})
	}
}
