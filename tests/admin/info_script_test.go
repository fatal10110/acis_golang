package admin

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The //info script scenario's NPC ids: one the WarriorAggressive behavior
// and the fixture quest are bound to, and one no script is bound to.
const (
	infoScriptedID   = int32(20012)
	infoUnscriptedID = int32(29990)
)

// infoScriptPlayer is the player the fixture quest binds its second timer
// to when an NPC it is bound to is created.
var infoScriptPlayer atomic.Pointer[attackable.Combatant]

// infoFixtureQuest is a real quest bound to infoScriptedID's attack, death,
// quest start and talk, and to its creation, when it starts two timers
// bound to the NPC: one bound to no player and one bound to
// infoScriptPlayer.
func infoFixtureQuest() script.Script {
	ids := []int32{infoScriptedID}
	return script.Script{
		QuestID: 901,
		Bind: script.Bindings{
			script.EventAttacked: ids, script.EventMyDying: ids, script.EventQuestStart: ids,
			script.EventTalked: ids, script.EventCreated: ids,
		},
		Hooks: script.Hooks{
			OnAttacked: func(*script.Script, script.Attacked) {},
			OnMyDying:  func(*script.Script, script.MyDying) {},
			OnTalk:     func(*script.Script, script.Talk) string { return "" },
			OnTimer:    func(*script.Script, script.Timer) string { return "" },
			OnCreated: func(s *script.Script, e script.Created) {
				s.StartTimer("info_tick", e.NPC, nil, time.Hour)
				if p := infoScriptPlayer.Load(); p != nil {
					s.StartTimer("info_chase", e.NPC, script.PlayerOf(*p), time.Hour)
				}
			},
		},
	}
}

// infoScripts are the fixture quest, then the behavior chain down to
// WarriorAggressive, in that list order.
func infoScripts() ([]script.Listing, script.Catalog) {
	catalog := script.Catalog{
		"quest.Q901_InfoFixture":                                                               infoFixtureQuest,
		"script.ai.individual.DefaultNpc":                                                      defaultnpc.New,
		"script.ai.individual.Monster.MonsterAI":                                               monster.New,
		"script.ai.individual.Monster.WarriorBase.WarriorBase":                                 warrior.Base,
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                             warrior.Warrior,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive": warrior.Aggressive,
	}
	var list []script.Listing
	for _, path := range []string{
		"quest.Q901_InfoFixture",
		"script.ai.individual.DefaultNpc",
		"script.ai.individual.Monster.MonsterAI",
		"script.ai.individual.Monster.WarriorBase.WarriorBase",
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior",
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive",
	} {
		list = append(list, script.Listing{Path: path})
	}
	return list, catalog
}

// infoScriptTemplate is a level 1 fixture monster of id named name.
func infoScriptTemplate(id int32, name string, params npc.AIParams) *npc.Template {
	return &npc.Template{
		ID: int(id), TemplateID: int(id), Type: "Monster", Name: name, Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, CollisionRadius: 10, CollisionHeight: 20,
		AIParams: params,
	}
}

// aiTabs is the tab row an //info ai page opens with, the page shown
// bracketed.
func aiTabs(page int) string {
	tabs := []string{"AI Path", "Template", "Spawn", "Npc"}
	var b strings.Builder
	b.WriteString("<center><table width=240><tr><td width=70>")
	for i, tab := range tabs {
		if i > 0 {
			b.WriteString("</td><td width=70>")
		}
		if i == page {
			b.WriteString("[" + tab + "]")
		} else {
			b.WriteString(`<a action="bypass -h admin_info ai ` + strconv.Itoa(i) + `">` + tab + "</a>")
		}
	}
	b.WriteString("</td></tr></table></center><br>")
	return b.String()
}

// scratchRows is the body of //info ai 3: ints are the i_ai0..4 and
// i_quest0..4 values in pairs, creatures the c_ai0..4 and c_quest0..4
// descriptions in pairs, then param1, flag, param2, respawnTime, param3
// and weightPoint.
func scratchRows(ints, creatures [10]string, tail [6]string) string {
	row := func(l, lv, r, rv string) string {
		return `<tr><td width=140><font color="LEVEL">[` + l + `]</font> ` + lv + `</td><td width=140 align=left><font color="LEVEL">[` + r + `]</font> ` + rv + `</td></tr>`
	}
	var b strings.Builder
	b.WriteString("<table width=280>")
	for i := range 5 {
		n := strconv.Itoa(i)
		b.WriteString(row("i_ai"+n, ints[i], "i_quest"+n, ints[5+i]))
	}
	b.WriteString("</table><br><table width=280>")
	for i := range 5 {
		n := strconv.Itoa(i)
		b.WriteString(row("c_ai"+n, creatures[i], "c_quest"+n, creatures[5+i]))
	}
	b.WriteString("</table><br><table width=280>")
	b.WriteString(row("param1", tail[0], "flag", tail[1]))
	b.WriteString(row("param2", tail[2], "respawnTime", tail[3]))
	b.WriteString(row("param3", tail[4], "weightPoint", tail[5]))
	b.WriteString("</table>")
	return b.String()
}

// teleportLink is the creature description of c: a teleport to where it
// stands, named name.
func teleportLink(c attackable.Combatant, name string) string {
	x, y, z := c.Position()
	return `<a action="bypass admin_teleport ` + strconv.Itoa(x) + " " + strconv.Itoa(y) + " " + strconv.Itoa(z) + " " + strconv.Itoa(c.Heading()) + `">` + name + "</a>"
}

// TestAdminInfoNpcScriptPages pins AdminInfo.java's script and AI pages
// (sendScriptInfos, sendAiInfos pages 0 and 3) on an NPC that a behavior
// and a quest are bound to, and on one no script is bound to: the scripts
// on each event list of the template, by event; the timers bound to the
// NPC by script, each with the player it is bound to; the class chain of
// the first script that is not a real quest; and the NPC's script memory,
// its creature slots as teleport links.
func TestAdminInfoNpcScriptPages(t *testing.T) {
	list, catalog := infoScripts()
	srv, gmID := bootAdmin(t, adminLevel,
		gameservertest.WithHTMLPages(shippedAdminPages(t, "npcinfo/default.htm", "npcinfo/script.htm")),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			// IsVs makes WarriorBase keep the NPC itself in c_ai0 at its
			// creation; ShoutMsg2 makes Warrior start its 1001 timer.
			infoScriptTemplate(infoScriptedID, "Aggressive Wolf Pup", npc.AIParams{"IsVs": "1", "ShoutMsg2": "1"}),
			infoScriptTemplate(infoUnscriptedID, "Quiet Wolf", nil),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{infoScriptedID: script.KindHostile, infoUnscriptedID: script.KindHostile}, list, catalog),
	)
	gm := srv.Client
	enterWorld(t, gm)
	admin := liveCombatant(t, srv, gmID)
	infoScriptPlayer.Store(&admin)
	t.Cleanup(func() { infoScriptPlayer.Store(nil) })

	spawn := func(id int32, dx int) *npc.Hostile {
		t.Helper()
		n, err := srv.NpcSpawns.AddSpawn(id, spawnX+dx, spawnY, spawnZ, 0, false, 0)
		if err != nil {
			t.Fatalf("spawn %d: %v", id, err)
		}
		srv.Settle(t)
		drain(t, gm)
		return n.(*npc.Hostile)
	}
	scripted := spawn(infoScriptedID, 40)
	unscripted := spawn(infoUnscriptedID, -40)

	// The scripted NPC.
	exchange(t, gm, encodeAction(scripted.ObjectID()))
	warriorOnly := "WarriorAggressive<br1>"
	both := "Q901_InfoFixture<br1>WarriorAggressive<br1>"
	event := func(name, scripts string) string {
		return `<br><font color="LEVEL">` + name + "</font><br1>" + scripts
	}
	assertInfoPage(t, gm, "info script", shippedPage(t, "npcinfo/script.htm",
		"%scripts%", event("ATTACKED", both)+
			event("ATTACK_FINISHED", warriorOnly)+
			event("CLAN_ATTACKED", warriorOnly)+
			event("CREATED", both)+
			event("MY_DYING", both)+
			event("NO_DESIRE", warriorOnly)+
			event("QUEST_START", "Q901_InfoFixture<br1>")+
			event("SEE_CREATURE", warriorOnly)+
			event("SEE_SPELL", warriorOnly)+
			event("TALKED", "Q901_InfoFixture<br1>")+
			event("USE_SKILL_FINISHED", warriorOnly)+
			event("SCRIPT_EVENT", warriorOnly),
		"%tasks%", event("Q901_InfoFixture", "info_chase affecting player Admin<br1>info_tick<br1>")+
			event("WarriorAggressive", "1001<br1>"),
	))

	// The first script that is not a real quest is WarriorAggressive, on
	// the attacked list after the quest; the AI path is its class chain.
	chain := aiTabs(0) + "WarriorAggressive<br1>Warrior<br1>WarriorBase<br1>MonsterAI<br1>DefaultNpc<br1>"
	for _, cmd := range []string{"info ai", "info ai 0", "info ai x", "info ai 4", "info ai -1", "info ai 99999999999"} {
		assertInfoPage(t, gm, cmd, defaultPage(t, chain))
	}

	mem := scripted.Scratch()
	mem.SetInt(npc.IntAI2, 7)
	mem.SetInt(npc.IntQuest4, -3)
	mem.SetInt(npc.IntParam1, 1000)
	mem.SetInt(npc.IntRespawnTime, 60)
	mem.SetCreature(npc.CreatureQuest1, admin)
	none := "none"
	assertInfoPage(t, gm, "info ai 3", defaultPage(t, aiTabs(3)+scratchRows(
		[10]string{"0", "0", "7", "0", "0", "0", "0", "0", "0", "-3"},
		[10]string{teleportLink(scripted, "Aggressiv..."), none, none, none, none, none, teleportLink(admin, "Admin"), none, none, none},
		[6]string{"1000", "0", "0", "60", "0", "1"},
	)))

	// The NPC no script is bound to.
	exchange(t, gm, encodeAction(unscripted.ObjectID()))
	assertInfoPage(t, gm, "info script", shippedPage(t, "npcinfo/script.htm",
		"%scripts%", "This NPC isn't affected by scripts.", "%tasks%", ""))
	assertInfoPage(t, gm, "info ai", defaultPage(t, aiTabs(0)+"This NPC doesn't hold any AI related script."))
	zeros := [10]string{"0", "0", "0", "0", "0", "0", "0", "0", "0", "0"}
	nones := [10]string{none, none, none, none, none, none, none, none, none, none}
	assertInfoPage(t, gm, "info ai 3", defaultPage(t, aiTabs(3)+scratchRows(zeros, nones, [6]string{"0", "0", "0", "0", "0", "1"})))

	// No template AI parameter, and a spawn of a script declaring none.
	assertInfoPage(t, gm, "info ai 1", defaultPage(t, strings.Replace(aiTabs(1), "[Template]", "[Template]</a>", 1)+"This NPC's template doesn't hold any AI parameters."))
	assertInfoPage(t, gm, "info ai 2", defaultPage(t, aiTabs(2)+"This NPC Spawn doesn't hold any memos."))
}
