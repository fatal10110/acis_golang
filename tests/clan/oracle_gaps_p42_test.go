package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Evidence for the alliance and war refusals the first parity sweep left
// without a scenario: create_ally while the clan is dissolving
// (VillageMaster.java createAlly), a war declaration refused for 30 wars,
// an allied target or a dissolving target (RequestStartPledgeWar.java), and
// a member without SP_CLAN_WAR starting, stopping or surrendering a war.
//
// Not covered here: a full personal surrender of a mutual war, fixed by
// #5324 (closing #5006), whose TestPersonalSurrenderEndsWar now asserts the
// surrenderer's side only, as the reference does; and the alliance
// dissolution refreshing clans that have no alliance, which only differs by
// redundant packets and is tracked on #5007.

// TestCreateAllyRefusedWhileDissolving has the leader of a clan under
// dissolution ask for an alliance: CANNOT_CREATE_ALLY_WHILE_DISSOLVING
// answers it alone and no alliance is stored.
func TestCreateAllyRefusedWhileDissolving(t *testing.T) {
	t.Parallel()
	future := strconv.FormatInt(time.Now().UnixMilli()+dayMs, 10)
	w := bootAllianceWorldSeeded(t, []string{
		`UPDATE clan_data SET dissolving_expiry_time = ` + future + ` WHERE clan_id = ` + itoa(knightsClanID),
	})

	frames := w.masterCommandBy(t, w.leader, "create_ally Ally")
	if ids := messages(t, frames); !slices.Equal(ids, []int{serverpackets.SystemMessageCannotCreateAllyWhileDissolving}) {
		t.Fatalf("create_ally while dissolving messages = %v, want [%d]", ids, serverpackets.SystemMessageCannotCreateAllyWhileDissolving)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_data WHERE ally_id <> 0`); got != 0 {
		t.Fatalf("clans holding an alliance = %d, want 0", got)
	}
}

// TestWarDeclarationRefusals has the founder declare war on Rivals under
// each refusal the reference answers with its own message: the clan at its
// 30 wars, the target in the clan's alliance and the target dissolving.
func TestWarDeclarationRefusals(t *testing.T) {
	t.Parallel()
	future := strconv.FormatInt(time.Now().UnixMilli()+dayMs, 10)
	var thirtyWars []string
	for i := range 30 {
		foe := itoa(rivalsClanID + 100 + int32(i))
		thirtyWars = append(thirtyWars,
			`INSERT INTO clan_data (clan_id, clan_name, clan_level, leader_id) VALUES (`+foe+`, 'Foe`+strconv.Itoa(i)+`', 1, `+itoa(rivalLeaderID)+`)`,
			`INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES (`+itoa(knightsClanID)+`, `+foe+`, 0)`)
	}
	for _, tc := range []struct {
		name string
		seed []string
		want int
	}{
		{"too many wars", thirtyWars, serverpackets.SystemMessageTooManyClanWars},
		{"allied target", []string{
			`UPDATE clan_data SET ally_id = ` + itoa(knightsClanID) + `, ally_name = 'Ally' WHERE clan_id IN (` + itoa(knightsClanID) + `, ` + itoa(rivalsClanID) + `)`,
		}, serverpackets.SystemMessageWarAgainstAlliedClan},
		{"dissolving target", []string{
			`UPDATE clan_data SET dissolving_expiry_time = ` + future + ` WHERE clan_id = ` + itoa(rivalsClanID),
		}, serverpackets.SystemMessageNoWarAgainstDissolvingClan},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := bootAllianceWorldSeeded(t, tc.seed, gameservertest.WithClanConfig(warConfig()))
			leader, rival := w.declare(t, "Rivals")
			wantMessages(t, "refused declaration", leader, tc.want)
			if len(rival) != 0 {
				t.Fatalf("target clan got %x, want nothing", opcodes(rival))
			}
			if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE clan1 = ? AND clan2 = ?`, knightsClanID, rivalsClanID); got != 0 {
				t.Fatalf("stored wars on Rivals = %d, want 0", got)
			}
		})
	}
}

// TestWarRequestsRefusedWithoutClanWarPrivilege has a plain member start,
// stop and surrender a war: each is answered YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT
// alone, and the war of its clan stands.
func TestWarRequestsRefusedWithoutClanWarPrivilege(t *testing.T) {
	t.Parallel()
	w := bootAllianceCast(t, []castMember{allySquire}, []string{
		squireInKnights(),
		`INSERT INTO clan_wars (clan1, clan2, expiry_time) VALUES (` + itoa(knightsClanID) + `, ` + itoa(rivalsClanID) + `, 0)`,
	}, gameservertest.WithClanConfig(warConfig()))
	squire := w.bringIn(t, allySquire)

	for _, tc := range []struct {
		what   string
		opcode byte
	}{
		{"start", clientpackets.OpcodeRequestStartPledgeWar},
		{"stop", clientpackets.OpcodeRequestStopPledgeWar},
		{"surrender", clientpackets.OpcodeRequestSurrenderPledgeWar},
	} {
		squire.Send(encodeWarName(tc.opcode, "Rivals"))
		wantMessages(t, "member's "+tc.what, drainFrames(t, squire), msgNotAuthorizedToDoThat)
	}
	if frames := drainFrames(t, w.member); len(frames) != 0 {
		t.Fatalf("Rivals got %x, want nothing", opcodes(frames))
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_wars WHERE clan1 = ? AND clan2 = ?`, knightsClanID, rivalsClanID); got != 1 {
		t.Fatalf("stored war on Rivals = %d, want it kept", got)
	}
}
