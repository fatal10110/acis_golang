// Package q234 is the quest Fate's Whispers, one of the two quests a
// subclass asks for: Maestro Reorin forges a level 75 character an A-grade
// weapon from a B-grade one, once it has brought him the Reiria soul orb,
// the three infernium scepters of the raid bosses' chests, the infernium
// varnish, his hammer and mold (the mold for Baium's blood on the pipette
// knife) and 984 B-grade crystals, and gives it the Star of Destiny.
package q234

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q234_FatesWhisper"

// Items.
const (
	reiriaSoulOrb            = 4666
	kermonInferniumScepter   = 4667
	golkondaInferniumScepter = 4668
	hallateInferniumScepter  = 4669

	inferniumVarnish = 4672
	reorinHammer     = 4670
	reorinMold       = 4671

	pipetteKnife    = 4665
	redPipetteKnife = 4673

	crystalB = 1460
)

// starOfDestiny is the reward.
const starOfDestiny = 5011

// NPCs.
const (
	reorin       = 31002
	cliff        = 30182
	ferris       = 30847
	zenkin       = 30178
	kaspar       = 30833
	cofferOfDead = 31027
	chestKernon  = 31028
	chestGolkon  = 31029
	chestHallate = 31030
)

// scepterOffset turns a raid boss chest's NPC id into the item id of the
// scepter it holds.
const scepterOffset = 26361

// baium is struck with the pipette knife.
const baium = 29020

// bloodCry is the NpcString Baium cries out when his blood is taken.
const bloodCry = 23434

// Raid bosses, each leaving its chest.
const (
	cabrio   = 25035
	kernon   = 25054
	golkonda = 25126
	hallate  = 25220
)

// chestDespawn is how long a raid boss's chest stays.
const chestDespawn = 120000 * time.Millisecond

// chests maps each raid boss to the chest it leaves.
func chests() map[int32]int32 {
	return map[int32]int32{cabrio: cofferOfDead, kernon: chestKernon, golkonda: chestGolkon, hallate: chestHallate}
}

// weapons are the B-grade weapons Reorin melts down, by item id.
func weapons() map[int32]string {
	return map[int32]string{
		79:   "Sword of Damascus",
		97:   "Lance",
		171:  "Deadman's Glory",
		175:  "Art of Battle Axe",
		210:  "Staff of Evil Spirits",
		234:  "Demon Dagger",
		268:  "Bellion Cestus",
		287:  "Bow of Peril",
		2626: "Samurai Dual-sword",
		7883: "Guardian Sword",
		7889: "Wizard's Tear",
		7893: "Kaim Vanul's Bones",
		7901: "Star Buster",
	}
}

// The quest variables of the weapon choice.
const (
	varBypass   = "bypass"
	varWeaponID = "weaponId"
)

// weaponNamePlaceholder is the placeholder of the weapon pages.
const weaponNamePlaceholder = "%weaponname%"

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Fate's Whispers",
		QuestID: 234,
		Items:   []int32{pipetteKnife, redPipetteKnife},
		Bind: script.Bindings{
			script.EventQuestStart: {reorin},
			script.EventTalked:     {reorin, cliff, ferris, zenkin, kaspar, chestKernon, chestGolkon, chestHallate, cofferOfDead},
			script.EventAttacked:   {baium},
			script.EventMyDying:    {cabrio, kernon, golkonda, hallate},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnAttacked: onAttacked, OnMyDying: onMyDying},
	}
}

// weaponPage is the page file filled with the name of the B-grade weapon
// id. An id that is not one of the weapons has no name to fill in, so the
// invocation aborts there.
func weaponPage(s *script.Script, file string, id int32) string {
	name, ok := weapons()[id]
	if !ok {
		panic("q234: no B-grade weapon " + strconv.Itoa(int(id)))
	}
	return strings.ReplaceAll(s.HTMLText(file), weaponNamePlaceholder, name)
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch {
	case strings.EqualFold(e.Name, "31002-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, "30182-01c.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, inferniumVarnish, 1)
	case strings.EqualFold(e.Name, "30178-01a.htm"):
		st.SetCond(6)
		s.PlaySound(p, script.SoundMiddle)
	case strings.EqualFold(e.Name, "30833-01b.htm"):
		st.SetCond(7)
		s.PlaySound(p, script.SoundMiddle)
		s.GiveItems(p, pipetteKnife, 1)
	case strings.HasPrefix(e.Name, "selectBGrade_"):
		if st.GetInt(varBypass) == 1 {
			return ""
		}
		st.Set(varWeaponID, strings.ReplaceAll(e.Name, "selectBGrade_", ""))
		htmltext = weaponPage(s, "31002-13.htm", st.GetInt(varWeaponID))
	case strings.HasPrefix(e.Name, "confirmWeapon"):
		st.Set(varBypass, strconv.Itoa(1))
		htmltext = weaponPage(s, "31002-14.htm", st.GetInt(varWeaponID))
	case strings.HasPrefix(e.Name, "selectAGrade_"):
		if st.GetInt(varBypass) != 1 {
			htmltext = "31002-16.htm"
			break
		}
		itemID := st.GetInt(varWeaponID)
		if !p.HasItem(itemID) {
			htmltext = weaponPage(s, "31002-15.htm", itemID)
			break
		}
		aGrade, err := commons.ParseInt(strings.ReplaceAll(e.Name, "selectAGrade_", ""), 32)
		if err != nil {
			panic(err)
		}
		aGradeItemID := int32(aGrade)
		htmltext = strings.ReplaceAll(s.HTMLText("31002-12.htm"), weaponNamePlaceholder, s.ItemName(aGradeItemID))
		s.TakeItems(p, itemID, 1)
		s.GiveItems(p, aGradeItemID, 1)
		s.GiveItems(p, starOfDestiny, 1)
		p.SocialAction(3)
		s.PlaySound(p, script.SoundFinish)
		st.Exit(false)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 75 {
			htmltext = "31002-01.htm"
		} else {
			htmltext = "31002-02.htm"
		}
	case questlog.StatusStarted:
		cond := st.Cond()
		switch npcID := e.NPC.NpcID(); npcID {
		case reorin:
			htmltext = talkReorin(s, p, st, cond)
		case cliff:
			if cond == 3 {
				if !p.HasItem(inferniumVarnish) {
					htmltext = "30182-01.htm"
				} else {
					htmltext = "30182-02.htm"
				}
			}
		case ferris:
			switch {
			case cond == 4 && !p.HasItem(reorinHammer):
				htmltext = "30847-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, reorinHammer, 1)
			case cond >= 4 && p.HasItem(reorinHammer):
				htmltext = "30847-02.htm"
			}
		case zenkin:
			switch {
			case cond == 5:
				htmltext = "30178-01.htm"
			case cond > 5:
				htmltext = "30178-02.htm"
			}
		case kaspar:
			switch {
			case cond == 6:
				htmltext = "30833-01.htm"
			case cond == 7:
				if p.HasItem(pipetteKnife) && !p.HasItem(redPipetteKnife) {
					htmltext = "30833-02.htm"
				} else {
					htmltext = "30833-03.htm"
					st.SetCond(8)
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, redPipetteKnife, 1)
					s.GiveItems(p, reorinMold, 1)
				}
			case cond > 7:
				htmltext = "30833-04.htm"
			}
		case cofferOfDead:
			if cond == 1 && !p.HasItem(reiriaSoulOrb) {
				htmltext = "31027-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, reiriaSoulOrb, 1)
			} else {
				htmltext = "31027-02.htm"
			}
		case chestKernon, chestGolkon, chestHallate:
			itemID := npcID - scepterOffset
			if cond == 2 && !p.HasItem(itemID) {
				htmltext = strconv.Itoa(int(npcID)) + "-01.htm"
				s.PlaySound(p, script.SoundItemGet)
				s.GiveItems(p, itemID, 1)
			} else {
				htmltext = strconv.Itoa(int(npcID)) + "-02.htm"
			}
		}
	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// talkReorin is Reorin's answer to a started quest at cond.
func talkReorin(s *script.Script, p *script.Player, st *script.QuestState, cond int32) string {
	switch {
	case cond == 1:
		if !p.HasItem(reiriaSoulOrb) {
			return "31002-04b.htm"
		}
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reiriaSoulOrb, 1)
		return "31002-05.htm"
	case cond == 2:
		if !p.HasItem(kermonInferniumScepter) || !p.HasItem(golkondaInferniumScepter) || !p.HasItem(hallateInferniumScepter) {
			return "31002-05c.htm"
		}
		st.SetCond(3)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, golkondaInferniumScepter, 1)
		s.TakeItems(p, hallateInferniumScepter, 1)
		s.TakeItems(p, kermonInferniumScepter, 1)
		return "31002-06.htm"
	case cond == 3:
		if !p.HasItem(inferniumVarnish) {
			return "31002-06b.htm"
		}
		st.SetCond(4)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, inferniumVarnish, 1)
		return "31002-07.htm"
	case cond == 4:
		if !p.HasItem(reorinHammer) {
			return "31002-07b.htm"
		}
		st.SetCond(5)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reorinHammer, 1)
		return "31002-08.htm"
	case cond > 4 && cond < 8:
		return "31002-08b.htm"
	case cond == 8:
		st.SetCond(9)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, reorinMold, 1)
		return "31002-09.htm"
	case cond == 9:
		if p.ItemCount(crystalB) < 984 {
			return "31002-09b.htm"
		}
		st.SetCond(10)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, crystalB, 984)
		return "31002-BGradeList.htm"
	case cond == 10:
		// A chosen weapon: the A-grade list once it is held.
		if st.GetInt(varBypass) == 1 {
			itemID := st.GetInt(varWeaponID)
			if p.HasItem(itemID) {
				return weaponPage(s, "31002-AGradeList.htm", itemID)
			}
			return weaponPage(s, "31002-15.htm", itemID)
		}
		return "31002-BGradeList.htm"
	}
	return script.NoQuestMsg()
}

func onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerCondition(p, e.NPC, 7)
	if st == nil {
		return
	}
	if p.ActiveWeaponItemID() == pipetteKnife && !p.HasItem(redPipetteKnife) {
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, pipetteKnife, 1)
		s.GiveItems(p, redPipetteKnife, 1)
		e.NPC.Say(script.NpcString(bloodCry))
	}
}

func onMyDying(s *script.Script, e script.MyDying) {
	s.Spawner().AddSpawn(chests()[e.NPC.NpcID()], e.NPC, true, chestDespawn)
}
