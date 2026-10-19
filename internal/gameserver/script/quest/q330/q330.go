// Package q330 is the quest Adept of Taste: Jonas sends the player to five
// suppliers for the ingredients of his contest dish, cooks it, and Mirien
// reviews it; the better the ingredients, the better the dish and the pay.
package q330

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q330_AdeptOfTaste"

// NPCs.
const (
	sonia   = 30062
	glyvka  = 30067
	rollant = 30069
	jacob   = 30073
	pano    = 30078
	mirien  = 30461
	jonas   = 30469
)

// Items.
const (
	ingredientList      = 1420
	soniaBotanyBook     = 1421
	redMandragoraRoot   = 1422
	whiteMandragoraRoot = 1423
	redMandragoraSap    = 1424
	whiteMandragoraSap  = 1425
	jacobInsectBook     = 1426
	nectar              = 1427
	royalJelly          = 1428
	honey               = 1429
	goldenHoney         = 1430
	panoContract        = 1431
	hobgoblinAmulet     = 1432
	dionianPotato       = 1433
	glyvkaBotanyBook    = 1434
	greenMarshMoss      = 1435
	brownMarshMoss      = 1436
	greenMossBundle     = 1437
	brownMossBundle     = 1438
	rolantCreatureBook  = 1439
	monsterEyeBody      = 1440
	monsterEyeMeat      = 1441
	jonasSteakDish1     = 1442
	jonasSteakDish2     = 1443
	jonasSteakDish3     = 1444
	jonasSteakDish4     = 1445
	jonasSteakDish5     = 1446
	mirienReview1       = 1447
	mirienReview2       = 1448
	mirienReview3       = 1449
	mirienReview4       = 1450
	mirienReview5       = 1451
)

// Rewards.
const (
	jonasSaladRecipe = 1455
	jonasSauceRecipe = 1456
	jonasSteakRecipe = 1457
)

// chances returns, by NPC id, the two bounds a kill's hundred-sided roll is
// read against: the first and the second ingredient of that monster.
func chances() map[int32][]int32 {
	m := make(map[int32][]int32, 6)
	m[20204] = []int32{92, 100}
	m[20229] = []int32{80, 95}
	m[20223] = []int32{70, 77}
	m[20154] = []int32{70, 77}
	m[20155] = []int32{87, 96}
	m[20156] = []int32{77, 85}
	return m
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Adept of Taste",
		QuestID: 330,
		Items: []int32{
			ingredientList, redMandragoraSap, whiteMandragoraSap, honey, goldenHoney, dionianPotato, greenMossBundle, brownMossBundle, monsterEyeMeat,
			mirienReview1, mirienReview2, mirienReview3, mirienReview4, mirienReview5,
			jonasSteakDish1, jonasSteakDish2, jonasSteakDish3, jonasSteakDish4, jonasSteakDish5,
			soniaBotanyBook, redMandragoraRoot, whiteMandragoraRoot, jacobInsectBook, nectar, royalJelly, panoContract, hobgoblinAmulet,
			glyvkaBotanyBook, greenMarshMoss, brownMarshMoss, rolantCreatureBook, monsterEyeBody,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {jonas},
			script.EventTalked:     {jonas, sonia, glyvka, rollant, jacob, pano, mirien},
			script.EventMyDying:    {20147, 20154, 20155, 20156, 20204, 20223, 20226, 20228, 20229, 20265, 20266},
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
	case strings.EqualFold(e.Name, "30469-03.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, ingredientList, 1)
	case strings.EqualFold(e.Name, "30062-05.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, soniaBotanyBook, 1)
		s.TakeItems(p, redMandragoraRoot, -1)
		s.TakeItems(p, whiteMandragoraRoot, -1)
		s.GiveItems(p, redMandragoraSap, 1)
	case strings.EqualFold(e.Name, "30073-05.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, jacobInsectBook, 1)
		s.TakeItems(p, nectar, -1)
		s.TakeItems(p, royalJelly, -1)
		s.GiveItems(p, honey, 1)
	case strings.EqualFold(e.Name, "30067-05.htm"):
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, glyvkaBotanyBook, 1)
		s.TakeItems(p, greenMarshMoss, -1)
		s.TakeItems(p, brownMarshMoss, -1)
		s.GiveItems(p, greenMossBundle, 1)
	}
	return htmltext
}

func onTalk(s *script.Script, e script.Talk) string {
	st := s.QuestState(e.Player, questName)
	htmltext := script.NoQuestMsg()
	if st == nil {
		return htmltext
	}
	p := e.Player
	switch st.Status() {
	case questlog.StatusCreated:
		if p.Level() < 24 {
			htmltext = "30469-01.htm"
		} else {
			htmltext = "30469-02.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case jonas:
			htmltext = talkJonas(s, st, p, htmltext)
		case mirien:
			htmltext = talkMirien(s, p, htmltext)
		case sonia:
			htmltext = talkSonia(s, p)
		case jacob:
			htmltext = talkJacob(s, p)
		case pano:
			htmltext = talkPano(s, p)
		case glyvka:
			htmltext = talkGlyvka(s, p)
		case rollant:
			htmltext = talkRollant(s, p)
		}
	}
	return htmltext
}

// talkJonas cooks the dish once every ingredient is in, and pays for
// Mirien's review of it. htmltext is the answer when nothing applies.
func talkJonas(s *script.Script, st *script.QuestState, p *script.Player, htmltext string) string {
	switch {
	case p.HasItem(ingredientList):
		if !hasAllIngredients(p) {
			return "30469-04.htm"
		}
		var dish int32
		specialIngredientsNumber := p.ItemCount(whiteMandragoraSap) + p.ItemCount(goldenHoney) + p.ItemCount(brownMossBundle)
		if s.RndBool() {
			htmltext = "30469-05t" + strconv.Itoa(int(specialIngredientsNumber+2)) + ".htm"
			dish = 1443 + specialIngredientsNumber
		} else {
			htmltext = "30469-05t" + strconv.Itoa(int(specialIngredientsNumber+1)) + ".htm"
			dish = 1442 + specialIngredientsNumber
		}
		// Sound according dish.
		if dish == jonasSteakDish5 {
			s.PlaySound(p, script.SoundJackpot)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
		s.TakeItems(p, ingredientList, 1)
		s.TakeItems(p, redMandragoraSap, 1)
		s.TakeItems(p, whiteMandragoraSap, 1)
		s.TakeItems(p, honey, 1)
		s.TakeItems(p, goldenHoney, 1)
		s.TakeItems(p, dionianPotato, 1)
		s.TakeItems(p, greenMossBundle, 1)
		s.TakeItems(p, brownMossBundle, 1)
		s.TakeItems(p, monsterEyeMeat, 1)
		s.GiveItems(p, dish, 1)
	case p.HasAtLeastOneItem(jonasSteakDish1, jonasSteakDish2, jonasSteakDish3, jonasSteakDish4, jonasSteakDish5):
		htmltext = "30469-06.htm"
	case p.HasAtLeastOneItem(mirienReview1, mirienReview2, mirienReview3, mirienReview4, mirienReview5):
		switch {
		case p.HasItem(mirienReview1):
			htmltext = "30469-06t1.htm"
			s.TakeItems(p, mirienReview1, 1)
			s.RewardItems(p, 57, 7500)
			s.RewardExpAndSp(p, 6000, 0)
		case p.HasItem(mirienReview2):
			htmltext = "30469-06t2.htm"
			s.TakeItems(p, mirienReview2, 1)
			s.RewardItems(p, 57, 9000)
			s.RewardExpAndSp(p, 7000, 0)
		case p.HasItem(mirienReview3):
			htmltext = "30469-06t3.htm"
			s.TakeItems(p, mirienReview3, 1)
			s.RewardItems(p, 57, 5800)
			s.GiveItems(p, jonasSaladRecipe, 1)
			s.RewardExpAndSp(p, 9000, 0)
		case p.HasItem(mirienReview4):
			htmltext = "30469-06t4.htm"
			s.TakeItems(p, mirienReview4, 1)
			s.RewardItems(p, 57, 6800)
			s.GiveItems(p, jonasSauceRecipe, 1)
			s.RewardExpAndSp(p, 10500, 0)
		case p.HasItem(mirienReview5):
			htmltext = "30469-06t5.htm"
			s.TakeItems(p, mirienReview5, 1)
			s.RewardItems(p, 57, 7800)
			s.GiveItems(p, jonasSteakRecipe, 1)
			s.RewardExpAndSp(p, 12000, 0)
		}
		s.PlaySound(p, script.SoundFinish)
		st.Exit(true)
	}
	return htmltext
}

// talkMirien reviews the dish the player brings. htmltext is the answer
// when nothing applies.
func talkMirien(s *script.Script, p *script.Player, htmltext string) string {
	switch {
	case p.HasItem(ingredientList):
		htmltext = "30461-01.htm"
	case p.HasAtLeastOneItem(jonasSteakDish1, jonasSteakDish2, jonasSteakDish3, jonasSteakDish4, jonasSteakDish5):
		s.PlaySound(p, script.SoundItemGet)
		switch {
		case p.HasItem(jonasSteakDish1):
			htmltext = "30461-02t1.htm"
			s.TakeItems(p, jonasSteakDish1, 1)
			s.GiveItems(p, mirienReview1, 1)
		case p.HasItem(jonasSteakDish2):
			htmltext = "30461-02t2.htm"
			s.TakeItems(p, jonasSteakDish2, 1)
			s.GiveItems(p, mirienReview2, 1)
		case p.HasItem(jonasSteakDish3):
			htmltext = "30461-02t3.htm"
			s.TakeItems(p, jonasSteakDish3, 1)
			s.GiveItems(p, mirienReview3, 1)
		case p.HasItem(jonasSteakDish4):
			htmltext = "30461-02t4.htm"
			s.TakeItems(p, jonasSteakDish4, 1)
			s.GiveItems(p, mirienReview4, 1)
		case p.HasItem(jonasSteakDish5):
			htmltext = "30461-02t5.htm"
			s.TakeItems(p, jonasSteakDish5, 1)
			s.GiveItems(p, mirienReview5, 1)
		}
	case p.HasAtLeastOneItem(mirienReview1, mirienReview2, mirienReview3, mirienReview4, mirienReview5):
		htmltext = "30461-04.htm"
	}
	return htmltext
}

// talkSonia hands out her botany book and turns forty roots of each colour
// into the white mandragora sap.
func talkSonia(s *script.Script, p *script.Player) string {
	if p.HasItem(redMandragoraSap) || p.HasItem(whiteMandragoraSap) {
		return "30062-07.htm"
	}
	if !p.HasItem(soniaBotanyBook) {
		s.GiveItems(p, soniaBotanyBook, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30062-01.htm"
	}
	switch {
	case p.ItemCount(redMandragoraRoot) < 40 || p.ItemCount(whiteMandragoraRoot) < 40:
		return "30062-02.htm"
	case p.ItemCount(whiteMandragoraRoot) >= 40:
		s.TakeItems(p, soniaBotanyBook, 1)
		s.TakeItems(p, redMandragoraRoot, -1)
		s.TakeItems(p, whiteMandragoraRoot, -1)
		s.GiveItems(p, whiteMandragoraSap, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30062-06.htm"
	default:
		return "30062-03.htm"
	}
}

// talkJacob hands out his insect book and makes honey from twenty nectars,
// golden honey when ten royal jellies come with them.
func talkJacob(s *script.Script, p *script.Player) string {
	if p.HasItem(honey) || p.HasItem(goldenHoney) {
		return "30073-07.htm"
	}
	if !p.HasItem(jacobInsectBook) {
		s.GiveItems(p, jacobInsectBook, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30073-01.htm"
	}
	switch {
	case p.ItemCount(nectar) < 20:
		return "30073-02.htm"
	case p.ItemCount(royalJelly) < 10:
		return "30073-03.htm"
	default:
		s.TakeItems(p, jacobInsectBook, 1)
		s.TakeItems(p, nectar, -1)
		s.TakeItems(p, royalJelly, -1)
		s.GiveItems(p, goldenHoney, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30073-06.htm"
	}
}

// talkPano hands out his contract and trades thirty hobgoblin amulets for a
// Dion potato.
func talkPano(s *script.Script, p *script.Player) string {
	if p.HasItem(dionianPotato) {
		return "30078-04.htm"
	}
	if !p.HasItem(panoContract) {
		s.GiveItems(p, panoContract, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30078-01.htm"
	}
	if p.ItemCount(hobgoblinAmulet) < 30 {
		return "30078-02.htm"
	}
	s.TakeItems(p, panoContract, 1)
	s.TakeItems(p, hobgoblinAmulet, -1)
	s.GiveItems(p, dionianPotato, 1)
	s.PlaySound(p, script.SoundItemGet)
	return "30078-03.htm"
}

// talkGlyvka hands out her botany book and turns twenty mosses of each
// colour into the brown moss bundle.
func talkGlyvka(s *script.Script, p *script.Player) string {
	if p.HasItem(greenMossBundle) || p.HasItem(brownMossBundle) {
		return "30067-07.htm"
	}
	if !p.HasItem(glyvkaBotanyBook) {
		s.GiveItems(p, glyvkaBotanyBook, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30067-01.htm"
	}
	switch {
	case p.ItemCount(greenMarshMoss) < 20 || p.ItemCount(brownMarshMoss) < 20:
		return "30067-02.htm"
	case p.ItemCount(brownMarshMoss) >= 20:
		s.TakeItems(p, glyvkaBotanyBook, 1)
		s.TakeItems(p, greenMarshMoss, -1)
		s.TakeItems(p, brownMarshMoss, -1)
		s.GiveItems(p, brownMossBundle, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30067-06.htm"
	default:
		return "30067-03.htm"
	}
}

// talkRollant hands out his creature book and trades thirty floating eye
// bodies for the meat.
func talkRollant(s *script.Script, p *script.Player) string {
	if p.HasItem(monsterEyeMeat) {
		return "30069-04.htm"
	}
	if !p.HasItem(rolantCreatureBook) {
		s.GiveItems(p, rolantCreatureBook, 1)
		s.PlaySound(p, script.SoundItemGet)
		return "30069-01.htm"
	}
	if p.ItemCount(monsterEyeBody) < 30 {
		return "30069-02.htm"
	}
	s.TakeItems(p, rolantCreatureBook, 1)
	s.TakeItems(p, monsterEyeBody, -1)
	s.GiveItems(p, monsterEyeMeat, 1)
	s.PlaySound(p, script.SoundItemGet)
	return "30069-03.htm"
}

func onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	if s.CheckPlayerState(p, e.NPC, questlog.StatusStarted) == nil {
		return
	}
	npcID := e.NPC.NpcID()
	switch npcID {
	case 20265:
		if p.HasItem(rolantCreatureBook) {
			count := int32(3)
			if s.Rnd(97) < 77 {
				count = 2
			}
			s.DropItems(p, monsterEyeBody, count, 30, 970000)
		}
	case 20266:
		if p.HasItem(rolantCreatureBook) {
			count := int32(2)
			if s.Rnd(10) < 7 {
				count = 1
			}
			s.DropItemsAlways(p, monsterEyeBody, count, 30)
		}
	case 20226:
		if p.HasItem(glyvkaBotanyBook) {
			moss := int32(brownMarshMoss)
			if s.Rnd(96) < 87 {
				moss = greenMarshMoss
			}
			s.DropItems(p, moss, 1, 20, 960000)
		}
	case 20228:
		if p.HasItem(glyvkaBotanyBook) {
			moss := int32(brownMarshMoss)
			if s.Rnd(10) < 9 {
				moss = greenMarshMoss
			}
			s.DropItemsAlways(p, moss, 1, 20)
		}
	case 20147:
		if p.HasItem(panoContract) {
			s.DropItemsAlways(p, hobgoblinAmulet, 1, 30)
		}
	case 20204, 20229:
		if p.HasItem(jacobInsectBook) {
			random := int32(s.Rnd(100))
			chances := chances()[npcID]
			if random < chances[0] {
				s.DropItemsAlways(p, nectar, 1, 20)
			} else if random < chances[1] {
				s.DropItemsAlways(p, royalJelly, 1, 10)
			}
		}
	case 20154, 20155, 20156, 20223:
		if p.HasItem(soniaBotanyBook) {
			random := int32(s.Rnd(100))
			chances := chances()[npcID]
			if random < chances[1] {
				root := int32(whiteMandragoraRoot)
				if random < chances[0] {
					root = redMandragoraRoot
				}
				s.DropItemsAlways(p, root, 1, 40)
			}
		}
	}
}

// hasAllIngredients reports whether p holds the five ingredients, either
// kind of each.
func hasAllIngredients(p *script.Player) bool {
	return p.HasItems(dionianPotato, monsterEyeMeat) && p.HasAtLeastOneItem(whiteMandragoraSap, redMandragoraSap) && p.HasAtLeastOneItem(goldenHoney, honey) && p.HasAtLeastOneItem(brownMossBundle, greenMossBundle)
}
