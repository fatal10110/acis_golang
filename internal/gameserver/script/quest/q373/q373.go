// Package q373 is the quest Supplier of Reagents: Wesley lends a mixing stone
// and a manual, and the Urn turns reagents and catalysts into oils and
// essences.
package q373

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q373_SupplierOfReagents"

// Quest variables.
const (
	ingredient = "ingredient"
	catalyst   = "catalyst"
)

// NPCs.
const (
	wesley = 30166
	urnNPC = 31149
)

// Quest items.
const (
	mixingStone  = 5904
	mixingManual = 6317
)

// Reagent pouches and boxes.
const (
	reagentPouch1 = 6007
	reagentPouch2 = 6008
	reagentPouch3 = 6009
	reagentBox    = 6010
)

// Ingredients.
const (
	wyrmsBlood     = 6011
	lavaStone      = 6012
	moonstoneShard = 6013
	rottenBone     = 6014
	demonsBlood    = 6015
	inferniumOre   = 6016
)

// Catalysts.
const (
	bloodRoot      = 6017
	volcanicAsh    = 6018
	quicksilver    = 6019
	sulfur         = 6020
	demonicEssence = 6031
	midnightOil    = 6030
)

// Products.
const (
	dracoplasm  = 6021
	magmaDust   = 6022
	moonDust    = 6023
	necroplasm  = 6024
	demonplasm  = 6025
	infernoDust = 6026
	fireEssence = 6028
	lunargent   = 6029
)

// Final products.
const (
	draconicEssence = 6027
	abyssOil        = 6032
	hellfireOil     = 6033
	nightmareOil    = 6034
	pureSilver      = 6320
)

// Monsters.
const (
	crendion               = 20813
	hallateMaid            = 20822
	hallateGuardian        = 21061
	platinumTribeShaman    = 20828
	platinumGuardianShaman = 21066
	lavaWyrm               = 21111
	hamesOrcShaman         = 21115
)

// formula is a mix: count of ingredient and one catalyst give the product.
type formula struct {
	count      int32
	ingredient int32
	catalyst   int32
	product    int32
}

// formulas lists every mix of the Urn.
func formulas() []formula {
	return []formula{
		{10, wyrmsBlood, bloodRoot, dracoplasm},
		{10, lavaStone, volcanicAsh, magmaDust},
		{10, moonstoneShard, volcanicAsh, moonDust},
		{10, rottenBone, bloodRoot, necroplasm},
		{10, demonsBlood, bloodRoot, demonplasm},
		{10, inferniumOre, volcanicAsh, infernoDust},
		{10, dracoplasm, quicksilver, draconicEssence},
		{10, magmaDust, sulfur, fireEssence},
		{10, moonDust, quicksilver, lunargent},
		{10, necroplasm, quicksilver, midnightOil},
		{10, demonplasm, sulfur, demonicEssence},
		{10, infernoDust, sulfur, abyssOil},
		{1, fireEssence, demonicEssence, hellfireOil},
		{1, lunargent, midnightOil, nightmareOil},
		{1, lunargent, quicksilver, pureSilver},
	}
}

// temperature is a heat level of the Urn: its chance, out of 100, to mix and the
// count of products a mix gives.
type temperature struct {
	index  int
	chance int
	count  int32
}

// temperatures lists the heat levels of the Urn.
func temperatures() []temperature {
	return []temperature{{1, 100, 1}, {2, 45, 3}, {3, 15, 5}}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Supplier of Reagents",
		QuestID: 373,
		Items:   []int32{mixingStone, mixingManual},
		Bind: script.Bindings{
			script.EventQuestStart: {wesley},
			script.EventTalked:     {wesley, urnNPC},
			script.EventMyDying: {
				crendion, hallateMaid, hallateGuardian, platinumTribeShaman,
				platinumGuardianShaman, lavaWyrm, hamesOrcShaman,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "30166-04.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, mixingStone, 1)
		s.GiveItems(p, mixingManual, 1)
	case strings.EqualFold(e.Name, "30166-09.htm"):
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	case strings.EqualFold(e.Name, "31149-02.htm"):
		if !p.HasItem(mixingStone) {
			htmltext = "31149-04.htm"
		}
	case strings.HasPrefix(e.Name, "31149-03-"):
		regentID := number(e.Name, 9, 13)
		for _, f := range formulas() {
			if f.ingredient != regentID {
				continue
			}
			// Not enough items, cancel the operation.
			if p.ItemCount(regentID) < f.count {
				break
			}
			st.Set(ingredient, strconv.Itoa(int(regentID)))
			return htmltext
		}
		htmltext = "31149-04.htm"
	case strings.HasPrefix(e.Name, "31149-06-"):
		catalystID := number(e.Name, 9, 13)
		// Not enough items, cancel the operation.
		if !p.HasItem(catalystID) {
			return "31149-04.htm"
		}
		st.Set(catalyst, strconv.Itoa(int(catalystID)))
	case strings.HasPrefix(e.Name, "31149-12-"):
		regent := st.GetInt(ingredient)
		catalystID := st.GetInt(catalyst)
		for _, f := range formulas() {
			if f.ingredient != regent || f.catalyst != catalystID {
				continue
			}
			// Not enough regents or catalysts.
			if p.ItemCount(regent) < f.count || !p.HasItem(catalystID) {
				break
			}
			s.TakeItems(p, regent, f.count)
			s.TakeItems(p, catalystID, 1)
			tempIndex := int(number(e.Name, 9, 10))
			for _, t := range temperatures() {
				if t.index != tempIndex {
					continue
				}
				if s.Rnd(100) < t.chance {
					s.GiveItems(p, f.product, t.count)
					return "31149-12-" + strconv.Itoa(int(f.product)) + ".htm"
				}
				return "31149-11.htm"
			}
		}
		htmltext = "31149-13.htm"
	}
	return htmltext
}

// number reads the characters of event from index from up to index to as a
// decimal integer; a missing or other character panics, so the invocation
// aborts.
func number(event string, from, to int) int32 {
	n, err := strconv.Atoi(event[from:to])
	if err != nil {
		panic(err)
	}
	return int32(n)
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	switch st.Status() {
	case questlog.StatusCreated:
		if e.Player.Level() < 57 {
			htmltext = "30166-01.htm"
		} else {
			htmltext = "30166-02.htm"
		}
	case questlog.StatusStarted:
		if e.NPC.NpcID() == wesley {
			htmltext = "30166-05.htm"
		} else {
			htmltext = "31149-01.htm"
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	p := st.Player()
	switch e.NPC.NpcID() {
	case platinumGuardianShaman:
		s.DropItems(p, reagentBox, 1, 0, 442000)
	case hamesOrcShaman:
		s.DropItems(p, reagentPouch3, 1, 0, 470000)
	case platinumTribeShaman:
		split(s, p, reagentPouch2, quicksilver, 680, 1000)
	case hallateMaid:
		split(s, p, reagentPouch1, volcanicAsh, 664, 844)
	case hallateGuardian:
		split(s, p, demonsBlood, moonstoneShard, 729, 833)
	case crendion:
		split(s, p, rottenBone, quicksilver, 618, 1000)
	case lavaWyrm:
		split(s, p, wyrmsBlood, lavaStone, 505, 750)
	}
}

// split draws once out of 1000: a roll at or above dropBelow drops nothing, a
// roll below firstBelow drops the first item and any other roll the second.
func split(s *script.Script, p *script.Player, first, second int32, firstBelow, dropBelow int) {
	random := s.Rnd(1000)
	if random < dropBelow {
		item := second
		if random < firstBelow {
			item = first
		}
		s.DropItemsAlways(p, item, 1, 0)
	}
}
