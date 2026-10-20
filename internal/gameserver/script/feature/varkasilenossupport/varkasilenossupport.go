// Package varkasilenossupport is the Varka Silenos village: its folk serve a
// Varka ally by the level of the alliance, a Varka soldier petrifies an ally
// who attacks it or casts a hostile spell near it, and an ally who kills one
// loses the alliance, a level of the alliance badge and the Varka missions.
package varkasilenossupport

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The village folk.
const (
	ashas  = 31377 // Hierarch
	naran  = 31378 // Messenger
	udan   = 31379 // Buffer
	diyabu = 31380 // Grocer
	hagos  = 31381 // Warehouse Keeper
	shikon = 31382 // Trader
	teranu = 31383 // Teleporter
)

// seed is the Nepenthes Seed Udan's blessings cost.
const seed = 7187

// varkas are the Varka soldiers.
func varkas() []int32 {
	return []int32{
		21350, 21351, 21353, 21354, 21355, 21357, 21358, 21360, 21361, 21362, 21369, 21370, 21364, 21365,
		21366, 21368, 21371, 21372, 21373, 21374, 21375,
	}
}

// buff is one of Udan's blessings: the skill, at level 1, and the seeds
// it costs.
type buff struct {
	id   skill.ID
	cost int32
}

// buffs are Udan's blessings, by the number of their link.
func buffs() []buff {
	return []buff{
		{4359, 2}, // Focus: Requires 2 Nepenthese Seeds
		{4360, 2}, // Death Whisper: Requires 2 Nepenthese Seeds
		{4345, 3}, // Might: Requires 3 Nepenthese Seeds
		{4355, 3}, // Acumen: Requires 3 Nepenthese Seeds
		{4352, 3}, // Berserker: Requires 3 Nepenthese Seeds
		{4354, 3}, // Vampiric Rage: Requires 3 Nepenthese Seeds
		{4356, 6}, // Empower: Requires 6 Nepenthese Seeds
		{4357, 6}, // Haste: Requires 6 Nepenthese Seeds
	}
}

// varkaQuests are the missions a broken alliance drops.
func varkaQuests() []string {
	return []string{
		"Q611_AllianceWithVarkaSilenos",
		"Q612_WarWithKetraOrcs",
		"Q613_ProveYourCourage",
		"Q614_SlayTheEnemyCommander",
		"Q615_MagicalPowerOfFire_Part1",
		"Q616_MagicalPowerOfFire_Part2",
	}
}

// petrify is the curse a soldier casts on an ally who turns on it.
func petrify() skill.Ref { return skill.Ref{ID: 4515, Level: 1} }

// New returns the feature.
func New() script.Script {
	soldiers := varkas()
	return script.Script{
		Dir: "feature",
		Bind: script.Bindings{
			script.EventFirstTalk:    {ashas, naran, udan, diyabu, hagos, shikon, teranu},
			script.EventTalked:       {udan, hagos, teranu},
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

// onAdvEvent answers Udan's blessing links, Hagos's withdrawal and Teranu's
// teleport. A blessing the player has the seeds for costs them and has
// Udan cast it on the player; the withdrawal opens the player's warehouse,
// unless it is empty; the teleport opens Teranu's standard destinations to
// an ally of level 4, every one to an ally of level 5.
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
		if p.ItemCount(seed) >= b.cost {
			htmltext = "31379-4.htm"
			s.TakeItems(p, seed, b.cost)
			e.NPC.AddCastDesire(p, skill.Ref{ID: b.id, Level: 1}, 1000000)
		}
	case e.Name == "Withdraw":
		if p.WarehouseSize() == 0 {
			htmltext = "31381-0.htm"
		} else {
			p.OpenWarehouseWithdraw()
		}
	case e.Name == "Teleport":
		switch p.VarkaKetraAlliance() {
		case -4:
			e.NPC.ShowTeleportWindow(p, travel.KindStandard)
			return ""
		case -5:
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
	case ashas:
		if allianceLevel < 0 {
			htmltext = "31377-friend.htm"
		} else {
			htmltext = "31377-no.htm"
		}
	case naran:
		if allianceLevel < 0 {
			htmltext = "31378-friend.htm"
		} else {
			htmltext = "31378-no.htm"
		}
	case udan:
		switch {
		case allianceLevel > -1:
			htmltext = "31379-3.htm"
		case allianceLevel > -3 && allianceLevel < 0:
			htmltext = "31379-1.htm"
		case allianceLevel < -2:
			if p.HasItem(seed) {
				htmltext = "31379-4.htm"
			} else {
				htmltext = "31379-2.htm"
			}
		}
	case diyabu:
		switch {
		case p.Karma() >= 1:
			htmltext = "31380-pk.htm"
		case allianceLevel >= 0:
			htmltext = "31380-no.htm"
		case allianceLevel == -1 || allianceLevel == -2:
			htmltext = "31380-1.htm"
		default:
			htmltext = "31380-2.htm"
		}
	case hagos:
		switch allianceLevel {
		case -1:
			htmltext = "31381-1.htm"
		case -2, -3:
			htmltext = "31381-2.htm"
		default:
			switch {
			case allianceLevel >= 0:
				htmltext = "31381-no.htm"
			case p.WarehouseSize() == 0:
				htmltext = "31381-3.htm"
			default:
				htmltext = "31381-4.htm"
			}
		}
	case shikon:
		switch allianceLevel {
		case -2:
			htmltext = "31382-1.htm"
		case -3, -4:
			htmltext = "31382-2.htm"
		case -5:
			htmltext = "31382-3.htm"
		default:
			htmltext = "31382-no.htm"
		}
	case teranu:
		switch {
		case allianceLevel >= 0:
			htmltext = "31383-no.htm"
		case allianceLevel < 0 && allianceLevel > -4:
			htmltext = "31383-1.htm"
		case allianceLevel == -4:
			htmltext = "31383-2.htm"
		default:
			htmltext = "31383-3.htm"
		}
	}

	return htmltext
}

// onAttacked petrifies a Varka ally who attacks the soldier, and forgets
// its attack.
func onAttacked(s *script.Script, e script.Attacked) {
	player := script.ActingPlayer(e.Attacker)
	if player != nil && player.AlliedWithVarka() {
		e.NPC.AddCastDesire(e.Attacker, petrify(), 1000000)
		e.NPC.RemoveAggro(e.Attacker)
	}
	var base script.Hooks
	base.Attacked(s, e)
}

// onClanAttacked has the called soldier petrify a Varka ally who attacks
// its clan, and forget its attack.
func onClanAttacked(s *script.Script, e script.ClanAttacked) {
	player := script.ActingPlayer(e.Attacker)
	if player != nil && player.AlliedWithVarka() {
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
				testVarkaDemote(s, member)
			}
		} else {
			testVarkaDemote(s, player)
		}
	}
	var base script.Hooks
	base.MyDying(s, e)
}

// onSeeSpell petrifies a Varka ally who casts a hostile spell the soldier
// sees, or the ally's summon when it cast it, and forgets its attack.
func onSeeSpell(s *script.Script, e script.SeeSpell) {
	if s.Skill(e.Skill).AggroPoints() > 0 && e.Caster.AlliedWithVarka() {
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

// testVarkaDemote breaks the alliance of a Varka ally: the level drops to
// none, the highest badge held is destroyed for the badge one level lower
// (the level 1 badge for nothing), and every Varka mission is dropped with
// its items.
func testVarkaDemote(s *script.Script, player *script.Player) {
	if !player.AlliedWithVarka() {
		return
	}
	// Drop the alliance (old friends become aggro).
	player.SetVarkaKetraAlliance(0)

	// Drop by 1 the level of that alliance (symbolized by a quest item).
	for i := int32(7225); i >= 7221; i-- {
		if player.HasItem(i) {
			// Destroy the badge.
			s.DestroyItems(player, i, player.FirstItemCount(i), true)

			// Badge lvl 1 ; no addition of badge of lower level.
			if i != 7221 {
				s.GiveItems(player, i-1, 1)
			}
			break
		}
	}

	for _, mission := range varkaQuests() {
		if pst := s.QuestState(player, mission); pst != nil {
			pst.Exit(true)
		}
	}
}
