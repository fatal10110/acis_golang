package quest

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestPrimevalSummonerBusyKeepsTheSecondPayersFee (#5157): while Vervato
// summons the bodies Payer paid for, Second, another member of the party
// with the fee and Fallen dead on the isle, is told to come back
// (vervato005.htm) before any fee is asked: Second keeps the adena, Payer
// paid once, and Payer's summon runs on unaffected: one cast, Fallen's body
// brought, vervato004.htm to Payer, and nothing more to Second.
func TestPrimevalSummonerBusyKeepsTheSecondPayersFee(t *testing.T) {
	t.Parallel()
	v := bootVervatoWorld(t, vervatoFee)
	second := v.srv.SeedCharacterFor(t, "second", "Second", 20, 0).ID
	v.srv.GiveItem(t, second, item.AdenaID, vervatoFee)
	sc := v.srv.DialClient(t, "second", 1)
	sc.Send(clientwire.RequestGameStart(0))
	readUntil(t, sc, serverpackets.OpcodeCharSelected)
	sc.Send(clientwire.SingleOpcode(clientpackets.OpcodeEnterWorld))
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

	var cast []string
	for range 3 {
		if err := v.srv.AI.Tick(); err != nil {
			t.Fatal(err)
		}
		for _, l := range v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client)) {
			if strings.HasPrefix(l, "S MagicSkillUse ") {
				cast = append(cast, l)
			}
		}
	}
	if want := fmt.Sprintf("S MagicSkillUse caster=vervato target=vervato skill=%d-1 ", corpseRevival); len(cast) != 1 || !strings.HasPrefix(cast[0], want) {
		t.Fatalf("Vervato's casts = %q, want one %q", cast, want)
	}

	v.srv.AdvanceUntil(t, "Fallen brought to Vervato", func() bool {
		x, y, _ := v.srv.PlayerPosition(t, v.fallen)
		return x == revivalX && y == revivalY
	})
	// See TestPrimevalSummonerBringsTheDeadPartyMembers for the wall-clock wait.
	done := []string{pageLine(t, "vervato004.htm"), "S ActionFailed"}
	var got []string
	if v.srv.DrivesClock() {
		v.srv.Advance(t, 3*time.Second)
		got = v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client))
	} else {
		v.srv.AdvanceUntil(t, "vervato004.htm", func() bool {
			got = append(got, v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client))...)
			return slices.Contains(got, done[0])
		})
	}
	if !slices.Equal(got, done) {
		t.Fatalf("Payer after the summon = %q, want %q", got, done)
	}

	// Second sees Vervato's cast like any bystander, and nothing else.
	var toSecond []string
	for _, l := range v.summonLines(t, v.srv.ReadQueued(t, sc)) {
		if !strings.HasPrefix(l, "S MagicSkillUse ") {
			toSecond = append(toSecond, l)
		}
	}
	if len(toSecond) != 0 {
		t.Fatalf("Second during Payer's summon = %q, want nothing", toSecond)
	}
	if got := v.srv.PlayerItemCount(t, second, item.AdenaID); got != vervatoFee {
		t.Fatalf("Second's adena after the summon = %d, want %d kept", got, vervatoFee)
	}
}
