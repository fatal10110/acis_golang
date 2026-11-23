package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// TestAllianceJoinAgainstSiegeRegistration pins #5146: Knights leads the
// alliance Ally and owns Gludio. Its leader invites Rivals; Rivals' leader
// accepts on its connection while Rivals asks to attack Gludio at the same
// time, or one after the other. Exactly one of the two passes: Rivals joins Ally and stays off the
// siege, or attacks Gludio outside Ally. The allies never stand on opposite
// sides.
func TestAllianceJoinAgainstSiegeRegistration(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"join first", "registration first", "together"} {
		t.Run(order, func(t *testing.T) {
			t.Parallel()
			allianceAgainstRegistration(t, order)
		})
	}
}

func allianceAgainstRegistration(t *testing.T, order string) {
	w := bootSiegeClanWorld(t, nil, []string{
		`UPDATE clan_data SET ally_id = ` + itoa(knightsClanID) + `, ally_name = 'Ally', hasCastle = 1 WHERE clan_id = ` + itoa(knightsClanID),
	})
	gludio, ok := w.srv.Sieges.Get(1)
	if !ok {
		t.Fatal("no Gludio siege")
	}
	if gludio.Side(knightsClanID) != siege.SideOwner {
		t.Fatalf("Knights stand %s on Gludio's siege, want OWNER", gludio.Side(knightsClanID))
	}
	rivals, ok := w.srv.Clans.Table().Get(rivalsClanID)
	if !ok {
		t.Fatal("Rivals not loaded")
	}
	w.leader.Send(encodeAllyTarget(clientpackets.OpcodeRequestJoinAlly, w.memberID))
	drainFrames(t, w.leader)
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinAlly); !ok {
		t.Fatal("Rivals' leader was not asked to join")
	}

	register := func() bool {
		_, refused := gludio.RegisterAttacker(rivals)
		return !refused
	}
	accept := func() {
		w.member.Send(encodeAllyTarget(clientpackets.OpcodeRequestAnswerJoinAlly, 1))
		w.srv.Settle(t)
	}
	var attacks bool
	switch order {
	case "join first":
		accept()
		attacks = register()
	case "registration first":
		attacks = register()
		accept()
	default:
		registered := make(chan bool)
		go func() { registered <- register() }()
		accept()
		attacks = <-registered
	}
	w.srv.Settle(t)
	drainFrames(t, w.member)

	allied := rivals.AllyID() == knightsClanID
	switch {
	case order == "join first" && !allied:
		t.Fatal("Rivals did not join Ally, uncontested")
	case order == "registration first" && !attacks:
		t.Fatal("Rivals' registration was refused, uncontested")
	case allied == attacks:
		t.Fatalf("Rivals allied %v and attacking %v, want exactly one", allied, attacks)
	}
	if allied && gludio.Side(rivalsClanID) != siege.SideNone {
		t.Fatalf("Rivals joined Ally and stand %s on Gludio's siege, want none", gludio.Side(rivalsClanID))
	}
	if attacks && gludio.Side(rivalsClanID) != siege.SideAttacker {
		t.Fatalf("Rivals' registration passed but they stand %s, want ATTACKER", gludio.Side(rivalsClanID))
	}
	want := int64(0)
	if allied {
		want = int64(knightsClanID)
	}
	if got := storedAlly(t, w, rivalsClanID); got.allyID != want {
		t.Fatalf("stored Rivals alliance = %+v, want ally id %d", got, want)
	}
}
