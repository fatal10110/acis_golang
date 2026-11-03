package quest

import (
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/scriptcontract"
)

// The handle operations Q234, Q235 and Q422 call (#3617), driven through a
// booted world as their hooks call them.

// x1Fists is the fists weapon of the booted class.
const x1Fists = 252

// x1ItemTemplates is opsItemTemplates plus the class's fists.
func x1ItemTemplates() gameservertest.Option {
	templates := gameservertest.ItemTemplates().All()
	for _, id := range append(slices.Clone(q001Items), dropItem, soulshotNoGrade, spiritshotNoGrade) {
		templates = append(templates, &item.Template{
			ID: id, Name: "Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	templates = append(templates, &item.Template{
		ID: x1Fists, Name: "Fist", Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
		Weapon: &item.WeaponDetail{Type: item.WeaponFist},
	})
	return gameservertest.WithItemTemplates(item.NewTable(templates))
}

// TestScriptHasItemsAndItemEnchantLevel pins ItemContainer.hasItems (every
// id held; an empty inventory holds nothing) and
// getItemByItemId(id).getEnchantLevel() (the first instance in inventory
// order; none held throws, so the invocation aborts).
func TestScriptHasItemsAndItemEnchantLevel(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, x1ItemTemplates())
	runOps(t, srv, objID, q001, func(sc *script.Script, p *script.Player) {
		empty := len(p.HeldItems()) == 0
		if got := p.HasItems(); got == empty {
			t.Errorf("HasItems() with %d items held = %v", len(p.HeldItems()), got)
		}
		if p.HasItems(dropItem) {
			t.Error("HasItems(dropItem) before any give = true")
		}
		sc.GiveItems(p, dropItem, 3)
		sc.GiveItemsEnchanted(p, swordID, 1, 3)
		sc.GiveItemsEnchanted(p, swordID, 1, 9)
		for _, tc := range []struct {
			ids  []int32
			want bool
		}{
			{[]int32{dropItem}, true},
			{[]int32{dropItem, swordID}, true},
			{[]int32{dropItem, swordID, soulshotNoGrade}, false},
			{[]int32{soulshotNoGrade}, false},
			{nil, true},
		} {
			if got := p.HasItems(tc.ids...); got != tc.want {
				t.Errorf("HasItems(%v) = %v, want %v", tc.ids, got, tc.want)
			}
		}
		// The inventory lists the newest instance first
		// (ItemInstance.compareTo: descending time, then object id).
		if got := p.ItemEnchantLevel(swordID); got != 9 {
			t.Errorf("ItemEnchantLevel(sword) = %d, want 9 (the sword given last)", got)
		}
		if got := p.ItemEnchantLevel(dropItem); got != 0 {
			t.Errorf("ItemEnchantLevel(dropItem) = %d, want 0", got)
		}
	})
	if srv.RunScript(t, objID, q001, func(_ *script.Script, p *script.Player) { p.ItemEnchantLevel(soulshotNoGrade) }) {
		t.Fatal("ItemEnchantLevel of an item the player does not hold returned")
	}
}

// TestScriptActiveWeaponItemID pins Player.getActiveWeaponItem().getItemId():
// the class's fists while the right hand is empty, the weapon once one is
// equipped there.
func TestScriptActiveWeaponItemID(t *testing.T) {
	t.Parallel()
	class := gameservertest.ClassTemplate()
	class.FistsItemID = x1Fists
	srv, objID := bootJournal(t, x1ItemTemplates(), gameservertest.WithClassTemplates(class))
	sword := srv.GiveItem(t, objID, swordID, 1)
	enterWorld(t, srv)

	weapon := func() int32 {
		var id int32
		runOps(t, srv, objID, q001, func(_ *script.Script, p *script.Player) { id = p.ActiveWeaponItemID() })
		return id
	}
	if got := weapon(); got != x1Fists {
		t.Fatalf("ActiveWeaponItemID() with an empty right hand = %d, want the fists %d", got, x1Fists)
	}
	srv.Client.Send(encodeUseItem(sword))
	srv.AdvanceUntil(t, "the sword is the active weapon", func() bool { return weapon() == swordID })
}

// TestScriptPkKillsSentToThePlayerOnly pins Player.setPkKills and
// sendPacket(new UserInfo(player)): the count reads back at once, the
// player alone gets one UserInfo, and a party mate standing by gets
// neither UserInfo nor CharInfo.
func TestScriptPkKillsSentToThePlayerOnly(t *testing.T) {
	t.Parallel()
	srv, leader, _, mateClient := partyPair(t, "1", "1")
	srv.ReadQueued(t, srv.Client)
	srv.ReadQueued(t, mateClient)

	run(t, srv, leader, func(_ *script.Script, p *script.Player) {
		if got := p.PkKills(); got != 0 {
			t.Errorf("PkKills() = %d before any set, want 0", got)
		}
		p.SetPkKills(7)
		if got := p.PkKills(); got != 7 {
			t.Errorf("PkKills() after SetPkKills(7) = %d", got)
		}
		p.SendUserInfo()
	})

	views := []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeCharInfo}
	if got := opcodesOf(framesOf(srv.ReadQueued(t, srv.Client), views...)); !slices.Equal(got, []byte{serverpackets.OpcodeUserInfo}) {
		t.Fatalf("player views %x, want one UserInfo", got)
	}
	if got := framesOf(srv.ReadQueued(t, mateClient), views...); len(got) != 0 {
		t.Fatalf("mate views %x, want none", opcodesOf(got))
	}
}

// TestScriptPkKillsSurviveRelog: a PK count a script sets is saved with
// the character, as the reference stores it with the character row.
func TestScriptPkKillsSurviveRelog(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, opsItemTemplates())
	run(t, srv, objID, func(_ *script.Script, p *script.Player) { p.SetPkKills(4) })
	restart(t, srv)
	enterWorld(t, srv)
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		if got := p.PkKills(); got != 4 {
			t.Errorf("PkKills() after relog = %d, want 4", got)
		}
	})
}

// The servitor the summon test calls; its id is the Sin Eater's, the
// summon Q422 checks for.
const x1ServitorID = 12564

// TestScriptActiveSummonAndSummonNpcID pins Player.getSummon (nil with no
// summon; the summon in the world otherwise) and Summon.getNpcId, which
// throws on anything but a summon.
func TestScriptActiveSummonAndSummonNpcID(t *testing.T) {
	t.Parallel()
	srv, objID := bootHelpers(t, opsItemTemplates(), gameservertest.WithNPCs(npc.NewTable([]*npc.Template{{
		ID: x1ServitorID, TemplateID: x1ServitorID, Type: "Servitor", Name: "Sin Eater", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CollisionRadius: 8, CollisionHeight: 20,
	}})))
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		if s := p.ActiveSummon(); s != nil {
			t.Errorf("ActiveSummon() with no summon = %v", s)
		}
	})
	srv.RunQuest(t, objID, q001, func(_ *script.Quests, c *player.Character, _ *script.Script) {
		c.SummonServitor(modelskill.Definition{ID: 1111, Level: 1, SkillType: "SUMMON", NpcID: x1ServitorID, SummonTotalLifeTime: 1_200_000})
	})
	var summonID int32
	srv.AdvanceUntil(t, "the servitor is in the world", func() bool {
		obj, ok := srv.State.Summon(objID)
		if ok {
			summonID = obj.ObjectID()
		}
		return ok
	})
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		s := p.ActiveSummon()
		if s == nil {
			t.Error("ActiveSummon() = nil with a servitor out")
			return
		}
		if s.ObjectID() != summonID || !s.Summon() || !s.Playable() {
			t.Errorf("ActiveSummon() = object %d summon %v playable %v, want object %d, a playable summon", s.ObjectID(), s.Summon(), s.Playable(), summonID)
		}
		if got := script.SummonNpcID(s); got != x1ServitorID {
			t.Errorf("SummonNpcID = %d, want %d", got, x1ServitorID)
		}
	})
	if srv.RunScript(t, objID, q001, func(_ *script.Script, p *script.Player) { script.SummonNpcID(p) }) {
		t.Fatal("SummonNpcID of a player returned")
	}
}

// The page text world: a real quest whose timer hook answers with its page
// 31002-13.htm filled with an item's name, as Q234 answers its weapon
// choice.
const (
	pageTextQuest = "Q961_PageText"
	pageTextFile  = "<html><body>%weaponname%: <a action=\"bypass -h Quest " + pageTextQuest + " confirmWeapon\">Yes</a></body></html>"
)

// TestScriptHTMLTextAndItemName pins Quest.getHtmlText (the page's text
// from the quest's own directory, with its line end; the missing-page
// notice naming the path) and ItemData.getTemplate(id).getName() (an
// unknown id throws). A hook answering the filled text shows it whole.
func TestScriptHTMLTextAndItemName(t *testing.T) {
	t.Parallel()
	fired := &atomic.Int32{}
	catalog := script.Catalog{
		"quest." + pageTextQuest: func() script.Script {
			return script.Script{Title: "Page Text", QuestID: 961, Hooks: script.Hooks{
				OnTimer: func(s *script.Script, _ script.Timer) string {
					fired.Add(1)
					return strings.ReplaceAll(s.HTMLText("31002-13.htm"), "%weaponname%", s.ItemName(swordID))
				},
			}}
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Pages", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(map[string]string{"script/quest/" + pageTextQuest + "/31002-13.htm": pageTextFile}),
		gameservertest.WithScripts([]script.Listing{{Path: "quest." + pageTextQuest}}, catalog),
	)
	objID := srv.SoleObjectID(t)
	enterWorld(t, srv)

	runOps(t, srv, objID, pageTextQuest, func(s *script.Script, p *script.Player) {
		if got, want := s.HTMLText("31002-13.htm"), pageTextFile+"\n"; got != want {
			t.Errorf("HTMLText(31002-13.htm) = %q, want %q", got, want)
		}
		missing := "<html><body>My html is missing:<br>./data/html/script/quest/" + pageTextQuest + "/none.htm</body></html>"
		if got := s.HTMLText("none.htm"); got != missing {
			t.Errorf("HTMLText(none.htm) = %q, want %q", got, missing)
		}
		if got := s.ItemName(swordID); got != "Sword" {
			t.Errorf("ItemName(sword) = %q, want Sword", got)
		}
		if !s.StartTimer("page", nil, p, 100*time.Millisecond) {
			t.Error("timer refused")
		}
	})
	srv.AdvanceUntil(t, "the page timer fires", func() bool { return fired.Load() > 0 })
	var got []string
	for _, f := range srv.ReadQueued(t, srv.Client) {
		if f[0] != serverpackets.OpcodeNpcHtmlMessage && f[0] != serverpackets.OpcodeActionFailed {
			continue
		}
		line, err := scriptcontract.Packet(f, nil)
		if err != nil {
			t.Fatalf("frame %x: %v", f, err)
		}
		got = append(got, line)
	}
	want := []string{
		"S NpcHtmlMessage obj=0 item=0 html=" + scriptcontract.Quote(strings.ReplaceAll(pageTextFile, "%weaponname%", "Sword")+"\n"),
		"S ActionFailed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("page answer:\n got %q\nwant %q", got, want)
	}
	if srv.RunScript(t, objID, pageTextQuest, func(s *script.Script, _ *script.Player) { s.ItemName(999999) }) {
		t.Fatal("ItemName of an unknown item returned")
	}
}
