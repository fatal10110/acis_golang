package quest

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPrimevalSummonerBusyKeepsTheSecondPayersFee (#5157): while Vervato
// summons the bodies Payer paid for, Second, another member of the party
// with the fee and Fallen dead on the isle, is told to come back
// (vervato005.htm) before any fee is asked: Second keeps the adena, and
// Payer paid once.
func TestPrimevalSummonerBusyKeepsTheSecondPayersFee(t *testing.T) {
	t.Parallel()
	v := bootVervatoWorld(t, vervatoFee)
	second := v.srv.SeedCharacterFor(t, "second", "Second", 20, 0).ID
	v.srv.GiveItem(t, second, item.AdenaID, vervatoFee)
	sc := v.srv.DialClient(t, "second", 1)
	sc.Send(encodeRequestGameStart(0))
	readUntil(t, sc, serverpackets.OpcodeCharSelected)
	sc.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	v.srv.ReadQueued(t, sc)
	v.srv.ReadQueued(t, v.srv.Client)

	v.joinParty(t)
	v.srv.Client.Send(encodeJoinParty("Second"))
	v.srv.ReadQueued(t, sc)
	sc.Send(encodeAnswerJoinParty(1))
	v.srv.ReadQueued(t, sc)
	v.srv.ReadQueued(t, v.srv.Client)
	v.srv.ReadQueued(t, v.fc)
	v.srv.MarkPlayerDead(t, v.fallen)

	paid := []string{fmt.Sprintf("S SystemMessage id=%d 1:%d", serverpackets.SystemMessageS1DisappearedAdena, vervatoFee), "S ActionFailed"}
	if got := v.ask(t); !slices.Equal(got, paid) {
		t.Fatalf("Payer paying = %q, want %q", got, paid)
	}

	obj := v.vervato.ObjectID()
	for range 2 {
		sc.Send(encodeTutorialAction(obj, v.at))
		v.srv.ReadQueued(t, sc)
	}
	sc.Send(encodeBypass(fmt.Sprintf("npc_%d_Quest PrimevalSummoner", obj)))
	busy := []string{pageLine(t, "vervato005.htm"), "S ActionFailed", "S ActionFailed"}
	if got := v.summonLines(t, v.srv.ReadQueued(t, sc)); !slices.Equal(got, busy) {
		t.Fatalf("Second asking during the summon = %q, want %q", got, busy)
	}
	if got := v.srv.PlayerItemCount(t, second, item.AdenaID); got != vervatoFee {
		t.Fatalf("Second's adena = %d, want %d kept", got, vervatoFee)
	}
	if got := v.adenaLeft(t); got != 0 {
		t.Fatalf("Payer's adena = %d, want 0 after one fee", got)
	}
}
