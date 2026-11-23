package clan

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// victor is the clan the siege's end announced victorious to the frames'
// recipient, "" for none.
func victor(t *testing.T, frames [][]byte) string {
	t.Helper()
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		if id, params := sysMsg(t, f); id == siege.MsgClanVictoriousOverSiege && len(params) > 0 {
			return params[0]
		}
	}
	return ""
}

// TestSealOfRulerAgainstSiegeEnd pins #5142: the Knights' leader casts Seal
// of Ruler on Gludio's only artifact while Gludio's siege runs, and the
// siege ends before the seal lands, after it, or as it lands. Whichever
// comes first, the castle's owner, the victor the end announces, the
// reputation it moves and the next siege's owner registration agree: an
// engraving landing after the end takes nothing, and one landing before
// it makes the end announce the Knights.
func TestSealOfRulerAgainstSiegeEnd(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"end first", "seal first", "together"} {
		t.Run(order, func(t *testing.T) {
			t.Parallel()
			w := engraveWorld(t)
			gludio, artifactID := gludioSiege(t, w)
			gludio.Start()
			startInWorld(t, w.leader)
			startInWorld(t, w.member)
			drainFrames(t, w.leader)
			drainFrames(t, w.member)
			w.srv.FlushPersistence(t)
			knightsRep := queryInt(t, w, "SELECT reputation_score FROM clan_data WHERE clan_id = ?", knightsClanID)

			w.leader.Send(encodeAction(artifactID, w.at))
			drainFrames(t, w.leader)
			w.leader.Send(encodeRequestMagicSkillUse(sealOfRulerID))
			if _, ok := firstOpcode(drainFrames(t, w.leader), serverpackets.OpcodeMagicSkillUse); !ok {
				t.Fatal("Seal of Ruler did not start")
			}
			drainFrames(t, w.member)

			switch order {
			case "end first":
				gludio.End()
				w.srv.Advance(t, 181*time.Second)
			case "seal first":
				w.srv.Advance(t, 181*time.Second)
				gludio.End()
			default:
				ended := make(chan struct{})
				go func() {
					defer close(ended)
					gludio.End()
				}()
				w.srv.Advance(t, 181*time.Second)
				<-ended
			}
			w.srv.Settle(t)
			if gludio.InProgress() {
				t.Fatal("the siege did not end")
			}
			lord := drainFrames(t, w.member)
			announced := victor(t, lord)
			owner := gludio.Castle().OwnerID()
			switch owner {
			case knightsClanID:
				if order == "end first" {
					t.Fatal("an engraving landing after the end took the castle")
				}
				if announced != "Knights" {
					t.Fatalf("the end announced %q victorious, want Knights, the new owner", announced)
				}
				if gludio.Side(knightsClanID) != siege.SideOwner || gludio.Side(rivalsClanID) != siege.SideNone {
					t.Fatalf("next siege: Knights %s, Rivals %s; want OWNER, none", gludio.Side(knightsClanID), gludio.Side(rivalsClanID))
				}
				w.srv.FlushPersistence(t)
				if got := queryInt(t, w, "SELECT reputation_score FROM clan_data WHERE clan_id = ?", knightsClanID); got != knightsRep+1000 {
					t.Fatalf("stored Knights reputation = %d, want %d", got, knightsRep+1000)
				}
				if n := queryInt(t, w, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", knightsClanID); n != 1 {
					t.Fatalf("stored Knights castle = %d, want 1", n)
				}
			case rivalsClanID:
				if order == "seal first" {
					t.Fatal("the seal landing before the end took nothing")
				}
				if announced != "Rivals" {
					t.Fatalf("the end announced %q victorious, want Rivals, the owner", announced)
				}
				if gludio.Side(rivalsClanID) != siege.SideOwner || gludio.Side(knightsClanID) != siege.SideNone {
					t.Fatalf("next siege: Rivals %s, Knights %s; want OWNER, none", gludio.Side(rivalsClanID), gludio.Side(knightsClanID))
				}
				w.srv.FlushPersistence(t)
				if n := queryInt(t, w, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", knightsClanID); n != 0 {
					t.Fatalf("stored Knights castle = %d, want 0", n)
				}
				if n := queryInt(t, w, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", rivalsClanID); n != 1 {
					t.Fatalf("stored Rivals castle = %d, want 1", n)
				}
			default:
				t.Fatalf("Gludio owner = %#x, want Knights or Rivals", owner)
			}
		})
	}
}
