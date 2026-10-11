package quest

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/primevalsummoner"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
	"github.com/rs/zerolog"
)

// Vervato (PrimevalSummoner), his fee and where he brings the bodies.
const (
	primevalPath   = "script.teleport.PrimevalSummoner"
	vervatoID      = 32104
	vervatoFee     = 200000
	corpseRevival  = 5121
	revivalX       = 11320
	revivalY       = -23504
	revivalZ       = -3640
	vervatoPageDir = "script/teleport/PrimevalSummoner/"
)

// vervatoSpawnlist places Vervato beside the character, the one NPC of an
// npcmaker whose territory holds them both and ends at x 400.
const vervatoSpawnlist = `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="isle" minZ="0" maxZ="100"><node x="0" y="0"/><node x="400" y="0"/><node x="400" y="400"/><node x="0" y="400"/></territory>
	<npcmaker name="isle_vervato" territory="isle" maximumNpcs="1">
		<ai type="default_maker"/>
		<npc id="32104" total="1" pos="60;20;30;0" respawn="1min"/>
	</npcmaker>
</list>`

// vervatoWorld is the character Payer in the world beside Vervato, with
// Fallen, a second player, in the world and in Payer's party.
type vervatoWorld struct {
	*dialogWorld
	vervato *npc.Folk
	fallen  int32
	fc      *testsupport.ScriptedClient
}

// datapackPage returns the datapack page rel (below data/html) as the page
// cache serves it.
func datapackPage(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(datapack.Path(t, append([]string{"data", "html"}, strings.Split(rel, "/")...)...))
	if err != nil {
		t.Fatal(err)
	}
	page := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	if !strings.HasSuffix(page, "\n") {
		page += "\n"
	}
	return page
}

func bootVervatoWorld(t *testing.T, adena int32) *vervatoWorld {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "isle.xml"), []byte(vervatoSpawnlist), 0o644); err != nil {
		t.Fatal(err)
	}
	makers, err := xmldata.LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{"default/32104.htm": datapackPage(t, "default/32104.htm")}
	for _, p := range []string{"vervato002.htm", "vervato003.htm", "vervato004.htm", "vervato005.htm"} {
		pages[vervatoPageDir+p] = datapackPage(t, vervatoPageDir+p)
	}
	var fallen int32
	w := bootSeamQuest(t, 20, pages, func(srv *gameservertest.Server, objID int32) {
		if adena > 0 {
			srv.GiveItem(t, objID, item.AdenaID, adena)
		}
		fallen = srv.SeedCharacterFor(t, "fallen", "Fallen", 20, 0).ID
	},
		gameservertest.WithAITask(),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{gameservertest.FolkTemplate("Folk", vervatoID)})),
		gameservertest.WithNpcSpawns(makers),
		gameservertest.WithNpcSpawnSkills(shippedSkillDefs(t)),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{vervatoID: script.KindFolk},
			[]script.Listing{{Path: primevalPath}}, script.Catalog{primevalPath: primevalsummoner.New}),
	)
	v := &vervatoWorld{dialogWorld: w, vervato: liveFolk(t, w.srv, vervatoID), fallen: fallen}
	w.roles[v.vervato.ObjectID()] = "vervato"

	c := w.srv.DialClient(t, "fallen", 1)
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
	w.srv.ReadQueued(t, c)
	v.fc = c
	w.srv.ReadQueued(t, w.srv.Client)
	return v
}

// joinParty invites Fallen into Payer's party.
func (v *vervatoWorld) joinParty(t *testing.T) {
	t.Helper()
	v.srv.Client.Send(encodeJoinParty("Fallen"))
	v.srv.ReadQueued(t, v.fc)
	v.fc.Send(encodeAnswerJoinParty(1))
	v.srv.ReadQueued(t, v.fc)
	v.srv.ReadQueued(t, v.srv.Client)
}

// summonLines renders the dialog, message and cast packets among frames.
func (v *vervatoWorld) summonLines(t *testing.T, frames [][]byte) []string {
	t.Helper()
	var out []string
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeMagicSkillUse:
			line, err := scriptcontract.Packet(f, v.roles)
			if err != nil {
				t.Fatalf("frame %x: %v", f, err)
			}
			out = append(out, line)
		}
	}
	return out
}

// ask talks to Vervato and follows his summon link.
func (v *vervatoWorld) ask(t *testing.T) []string {
	t.Helper()
	v.interact(t, v.vervato.ObjectID())
	v.srv.Client.Send(encodeBypass(fmt.Sprintf("npc_%d_Quest PrimevalSummoner", v.vervato.ObjectID())))
	return v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client))
}

// pageLine is the rendered NpcHtmlMessage of Vervato's page file.
func pageLine(t *testing.T, file string) string {
	t.Helper()
	page := strings.ReplaceAll(datapackPage(t, vervatoPageDir+file), "%objectId%", "{vervato}")
	return "S NpcHtmlMessage obj=vervato item=0 html=" + scriptcontract.Quote(page)
}

// adenaLeft returns Payer's adena.
func (v *vervatoWorld) adenaLeft(t *testing.T) int {
	t.Helper()
	return v.srv.PlayerItemCount(t, v.player, item.AdenaID)
}

// TestPrimevalSummonerTurnsAwayWithoutADeadPartyMember
// (PrimevalSummoner.onTalk): with no party, and with a party whose member
// is alive, Vervato has no body to summon: vervato002.htm, nothing paid.
func TestPrimevalSummonerTurnsAwayWithoutADeadPartyMember(t *testing.T) {
	t.Parallel()
	v := bootVervatoWorld(t, vervatoFee)
	want := []string{pageLine(t, "vervato002.htm"), "S ActionFailed", "S ActionFailed"}
	if got := v.ask(t); !slices.Equal(got, want) {
		t.Fatalf("with no party = %q, want %q", got, want)
	}
	v.joinParty(t)
	if got := v.ask(t); !slices.Equal(got, want) {
		t.Fatalf("with a living party member = %q, want %q", got, want)
	}
	if got := v.adenaLeft(t); got != vervatoFee {
		t.Fatalf("adena = %d, want %d untouched", got, vervatoFee)
	}
}

// TestPrimevalSummonerBringsTheDeadPartyMembers (PrimevalSummoner.onTalk,
// onUseSkillFinished, onTimer): with Fallen dead on the isle, Payer short
// of the fee is told so (vervato003.htm). Paying it answers no page: the
// adena go, Vervato casts Corpse Revival on himself, and as the cast ends
// Fallen's body is brought to his side. Asking again during the summon
// takes the fee again and answers vervato005.htm. Three seconds after the
// cast, Payer is shown vervato004.htm, and Vervato is free again: a dead
// member outside the isle is no body for him.
func TestPrimevalSummonerBringsTheDeadPartyMembers(t *testing.T) {
	t.Parallel()
	v := bootVervatoWorld(t, vervatoFee-1)
	v.joinParty(t)
	v.srv.MarkPlayerDead(t, v.fallen)

	want := []string{
		fmt.Sprintf("S SystemMessage id=%d", serverpackets.SystemMessageYouNotEnoughAdena),
		pageLine(t, "vervato003.htm"), "S ActionFailed", "S ActionFailed",
	}
	if got := v.ask(t); !slices.Equal(got, want) {
		t.Fatalf("short of the fee = %q, want %q", got, want)
	}
	if got := v.adenaLeft(t); got != vervatoFee-1 {
		t.Fatalf("adena short of the fee = %d, want %d", got, vervatoFee-1)
	}

	v.srv.RunScript(t, v.player, "PrimevalSummoner", func(s *script.Script, p *script.Player) {
		s.GiveItems(p, item.AdenaID, vervatoFee+1)
	})
	v.srv.ReadQueued(t, v.srv.Client)
	paid := []string{fmt.Sprintf("S SystemMessage id=%d 1:%d", serverpackets.SystemMessageS1DisappearedAdena, vervatoFee), "S ActionFailed"}
	if got := v.ask(t); !slices.Equal(got, paid) {
		t.Fatalf("paying = %q, want %q", got, paid)
	}
	if got := v.adenaLeft(t); got != vervatoFee {
		t.Fatalf("adena after paying = %d, want %d", got, vervatoFee)
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
	// The hit time is the skill's 500 scaled by the fixture's casting speed.
	if want := fmt.Sprintf("S MagicSkillUse caster=vervato target=vervato skill=%d-1 ", corpseRevival); len(cast) != 1 || !strings.HasPrefix(cast[0], want) {
		t.Fatalf("Vervato's casts = %q, want one %q", cast, want)
	}

	busy := []string{
		fmt.Sprintf("S SystemMessage id=%d 1:%d", serverpackets.SystemMessageS1DisappearedAdena, vervatoFee),
		pageLine(t, "vervato005.htm"), "S ActionFailed", "S ActionFailed",
	}
	if got := v.ask(t); !slices.Equal(got, busy) {
		t.Fatalf("asking during the summon = %q, want %q", got, busy)
	}
	if got := v.adenaLeft(t); got != 0 {
		t.Fatalf("adena after asking during the summon = %d, want 0", got)
	}

	v.srv.AdvanceUntil(t, "Fallen brought to Vervato", func() bool {
		x, y, _ := v.srv.PlayerPosition(t, v.fallen)
		return x == revivalX && y == revivalY
	})
	if !v.srv.PlayerDead(t, v.fallen) {
		t.Fatal("Fallen came back alive, want his body brought")
	}
	if got := v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client)); len(got) != 0 {
		t.Fatalf("Payer as the summon ends = %q, want nothing yet", got)
	}

	v.srv.Advance(t, 3*time.Second)
	done := []string{pageLine(t, "vervato004.htm"), "S ActionFailed"}
	if got := v.summonLines(t, v.srv.ReadQueued(t, v.srv.Client)); !slices.Equal(got, done) {
		t.Fatalf("Payer three seconds after the summon = %q, want %q", got, done)
	}

	// Fallen lies at the revival point, outside the isle: no body for
	// Vervato, who is free again and answers at once.
	if got := v.ask(t); !slices.Equal(got, []string{pageLine(t, "vervato002.htm"), "S ActionFailed", "S ActionFailed"}) {
		t.Fatalf("with the member's body off the isle = %q, want vervato002.htm", got)
	}
}
