// Package ketraorcsupport is the Ketra Orc village: its folk serve a Ketra
// ally by the level of the alliance, a Ketra soldier petrifies an ally who
// attacks it or casts a hostile spell near it, and an ally who kills one
// loses the alliance, a level of the alliance badge and the Ketra
// missions.
package ketraorcsupport

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The village folk.
const (
	kadun  = 31370 // Hierarch
	wahkan = 31371 // Messenger
	asefa  = 31372 // Soul Guide
	atan   = 31373 // Grocer
	jaff   = 31374 // Warehouse Keeper
	jumara = 31375 // Trader
	kurfa  = 31376 // Gate Keeper
)

// horn is the Buffalo Horn Asefa's blessings cost.
const horn = 7186

// ketras are the Ketra soldiers.
func ketras() []int32 {
	return []int32{
		21324, 21325, 21327, 21328, 21329, 21331, 21332, 21334, 21335, 21336, 21338, 21339, 21340, 21342,
		21343, 21344, 21345, 21346, 21347, 21348, 21349,
	}
}

// buff is one of Asefa's blessings: the skill, at level 1, and the horns
// it costs.
type buff struct {
	id   skill.ID
	cost int32
}

// buffs are Asefa's blessings, by the number of their link.
func buffs() []buff {
	return []buff{
		{4359, 2}, // Focus: Requires 2 Buffalo Horns
		{4360, 2}, // Death Whisper: Requires 2 Buffalo Horns
		{4345, 3}, // Might: Requires 3 Buffalo Horns
		{4355, 3}, // Acumen: Requires 3 Buffalo Horns
		{4352, 3}, // Berserker: Requires 3 Buffalo Horns
		{4354, 3}, // Vampiric Rage: Requires 3 Buffalo Horns
		{4356, 6}, // Empower: Requires 6 Buffalo Horns
		{4357, 6}, // Haste: Requires 6 Buffalo Horns
	}
}

// ketraQuests are the missions a broken alliance drops.
func ketraQuests() []string {
	return []string{
		"Q605_AllianceWithKetraOrcs",
		"Q606_WarWithVarkaSilenos",
		"Q607_ProveYourCourage",
		"Q608_SlayTheEnemyCommander",
		"Q609_MagicalPowerOfWater_Part1",
		"Q610_MagicalPowerOfWater_Part2",
	}
}

// petrify is the curse a soldier casts on an ally who turns on it.
func petrify() skill.Ref { return skill.Ref{ID: 4515, Level: 1} }

// New returns the feature.
func New() script.Script {
	soldiers := ketras()
	return script.Script{
		Dir: "feature",
		Bind: script.Bindings{
			script.EventFirstTalk:    {kadun, wahkan, asefa, atan, jaff, jumara, kurfa},
			script.EventTalked:       {asefa, jaff, kurfa},
			script.EventAttacked:     soldiers,
			script.EventClanAttacked: soldiers,
			script.EventMyDying:      soldiers,
			script.EventSeeSpell:     soldiers,
		},
		Hooks: script.Hooks{
			OnEvent:        onAdvEvent,
			OnFirstTalk:    onFirstTalk,
			OnAttacked:     onAttacked,
			OnClanAttacked: onClanAttacked,
			OnMyDying:      onMyDying,
			OnSeeSpell:     onSeeSpell,
		},
	}
}

// onAdvEvent answers Asefa's blessing links, Jaff's withdrawal and Kurfa's
// teleport. A blessing the player has the horns for costs them and has
// Asefa cast it on the player; the withdrawal opens the player's
// warehouse, unless it is empty; the teleport opens Kurfa's standard
// destinations to an ally of level 4, every one to an ally of level 5.
// Anything else, and a refusal, answers the no-quest page.
func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := script.NoQuestMsg()
	p := e.Player

	switch {
	case script.IsDigit(e.Name):
		// A number outside the int32 range, or past the last blessing,
		// aborts the event here.
		n, err := strconv.ParseInt(e.Name, 10, 32)
		if err != nil {
			panic(err)
		}
		b := buffs()[n]
		if p.ItemCount(horn) >= b.cost {
			htmltext = "31372-4.htm"
			s.TakeItems(p, horn, b.cost)
			e.NPC.AddCastDesire(p, skill.Ref{ID: b.id, Level: 1}, 1000000)
		}
	case e.Name == "Withdraw":
		if p.WarehouseSize() == 0 {
			htmltext = "31374-0.htm"
		} else {
			p.OpenWarehouseWithdraw()
		}
	case e.Name == "Teleport":
		switch p.VarkaKetraAlliance() {
		case 4:
			e.NPC.ShowTeleportWindow(p, travel.KindStandard)
			return ""
		case 5:
			e.NPC.ShowTeleportWindow(p, travel.KindAlly)
			return ""
		}
	}

	return htmltext
}

// onFirstTalk answers each of the folk by the player's alliance level.
func onFirstTalk(_ *script.Script, e script.FirstTalk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player

	allianceLevel := p.VarkaKetraAlliance()

	switch e.NPC.NpcID() {
	case kadun:
		if allianceLevel > 0 {
			htmltext = "31370-friend.htm"
		} else {
			htmltext = "31370-no.htm"
		}
	case wahkan:
		if allianceLevel > 0 {
			htmltext = "31371-friend.htm"
		} else {
			htmltext = "31371-no.htm"
		}
	case asefa:
		switch {
		case allianceLevel < 1:
			htmltext = "31372-3.htm"
		case allianceLevel < 3 && allianceLevel > 0:
			htmltext = "31372-1.htm"
		case allianceLevel > 2:
			if p.HasItem(horn) {
				htmltext = "31372-4.htm"
			} else {
				htmltext = "31372-2.htm"
			}
		}
	case atan:
		switch {
		case p.Karma() >= 1:
			htmltext = "31373-pk.htm"
		case allianceLevel <= 0:
			htmltext = "31373-no.htm"
		case allianceLevel == 1 || allianceLevel == 2:
			htmltext = "31373-1.htm"
		default:
			htmltext = "31373-2.htm"
		}
	case jaff:
		switch allianceLevel {
		case 1:
			htmltext = "31374-1.htm"
		case 2, 3:
			htmltext = "31374-2.htm"
		default:
			switch {
			case allianceLevel <= 0:
				htmltext = "31374-no.htm"
			case p.WarehouseSize() == 0:
				htmltext = "31374-3.htm"
			default:
				htmltext = "31374-4.htm"
			}
		}
	case jumara:
		switch allianceLevel {
		case 2:
			htmltext = "31375-1.htm"
		case 3, 4:
			htmltext = "31375-2.htm"
		case 5:
			htmltext = "31375-3.htm"
		default:
			htmltext = "31375-no.htm"
		}
	case kurfa:
		switch {
		case allianceLevel <= 0:
			htmltext = "31376-no.htm"
		case allianceLevel > 0 && allianceLevel < 4:
			htmltext = "31376-1.htm"
		case allianceLevel == 4:
			htmltext = "31376-2.htm"
		default:
			htmltext = "31376-3.htm"
		}
	}

	return htmltext
}

// onAttacked petrifies a Ketra ally who attacks the soldier, and forgets
// its attack.
func onAttacked(s *script.Script, e script.Attacked) {
	player := script.ActingPlayer(e.Attacker)
	if player != nil && player.AlliedWithKetra() {
		e.NPC.AddCastDesire(e.Attacker, petrify(), 1000000)
		e.NPC.RemoveAggro(e.Attacker)
	}
	var base script.Hooks
	base.Attacked(s, e)
}

// onClanAttacked has the called soldier petrify a Ketra ally who attacks
// its clan, and forget its attack.
func onClanAttacked(s *script.Script, e script.ClanAttacked) {
	player := script.ActingPlayer(e.Attacker)
	if player != nil && player.AlliedWithKetra() {
		e.Called.AddCastDesire(e.Attacker, petrify(), 1000000)
		e.Called.RemoveAggro(e.Attacker)
	}
	var base script.Hooks
	base.ClanAttacked(s, e)
}

// onMyDying demotes the killer, or every member of the killer's party.
func onMyDying(s *script.Script, e script.MyDying) {
	player := script.ActingPlayer(e.Killer)
	if player != nil {
		if members := player.PartyMembers(); members != nil {
			for _, member := range members {
				testKetraDemote(s, member)
			}
		} else {
			testKetraDemote(s, player)
		}
	}
	var base script.Hooks
	base.MyDying(s, e)
}

// onSeeSpell petrifies a Ketra ally who casts a hostile spell the soldier
// sees, or the ally's summon when it cast it, and forgets its attack.
func onSeeSpell(s *script.Script, e script.SeeSpell) {
	if s.Skill(e.Skill).AggroPoints() > 0 && e.Caster.AlliedWithKetra() {
		var trueCaster script.Creature = e.Caster
		if e.IsPet {
			if summon := e.Caster.ActiveSummon(); summon != nil {
				trueCaster = summon
			}
		}

		e.NPC.AddCastDesire(trueCaster, petrify(), 1000000)
		e.NPC.RemoveAggro(trueCaster)
	}
	var base script.Hooks
	base.SeeSpell(s, e)
}

// testKetraDemote breaks the alliance of a Ketra ally: the level drops to
// none, the highest badge held is destroyed for the badge one level lower
// (the level 1 badge for nothing), and every Ketra mission is dropped with
// its items.
func testKetraDemote(s *script.Script, player *script.Player) {
	if !player.AlliedWithKetra() {
		return
	}
	// Drop the alliance (old friends become aggro).
	player.SetVarkaKetraAlliance(0)

	// Drop by 1 the level of that alliance (symbolized by a quest item).
	for i := int32(7215); i >= 7211; i-- {
		if player.HasItem(i) {
			// Destroy the badge.
			s.DestroyItems(player, i, player.FirstItemCount(i), true)

			// Badge lvl 1 ; no addition of badge of lower level.
			if i != 7211 {
				s.GiveItems(player, i-1, 1)
			}
			break
		}
	}

	for _, mission := range ketraQuests() {
		if pst := s.QuestState(player, mission); pst != nil {
			pst.Exit(true)
		}
	}
}
