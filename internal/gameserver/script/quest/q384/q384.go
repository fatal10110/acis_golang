// Package q384 is the quest Warehouse Keeper's Pastime: Cliff trades the
// medals of the monsters for a game of picking six cells of a scrambled
// board, and pays by the lines the picks complete.
package q384

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

const questName = "Q384_WarehouseKeepersPastime"

// Npcs.
const (
	cliff = 30182
	baxt  = 30685
)

// Quest item.
const medal = 5964

// reward is an item given when the draw is below chance.
type reward struct {
	chance int
	item   int32
}

// chances returns the medal drop chance of each monster, by NPC id.
func chances() map[int32]int32 {
	return map[int32]int32{
		20947: 160000, // Connabi
		20948: 180000, // Bartal
		20945: 120000, // Cadeine
		20946: 150000, // Sanhidro
		20635: 150000, // Carinkain
		20773: 610000, // Conjurer Bat Lord
		20774: 600000, // Conjurer Bat
		20760: 240000, // Dragon Bearer Archer
		20758: 240000, // Dragon Bearer Chief
		20759: 230000, // Dragon Bearer Warrior
		20242: 220000, // Dustwind Gargoyle
		20281: 220000, // Dustwind Gargoyle (2)
		20556: 140000, // Giant Monstereye
		20668: 210000, // Grave Guard
		20241: 220000, // Hunter Gargoyle
		20286: 220000, // Hunter Gargoyle (2)
		20949: 190000, // Luminun
		20950: 200000, // Innersen
		20942: 90000,  // Nightmare Guide
		20943: 120000, // Nightmare Keeper
		20944: 110000, // Nightmare Lord
		20559: 140000, // Rotting Golem
		20243: 210000, // Thunder Wyrm
		20282: 210000, // Thunder Wyrm (2)
		20677: 340000, // Tulben
		20605: 150000, // Weird Drake
	}
}

func rewards10Win() []reward {
	return []reward{
		{16, 1888}, // Synthetic Cokes
		{32, 1887}, // Varnish of Purity
		{50, 1894}, // Crafted Leather
		{80, 952},  // Scroll: Enchant Armor (C)
		{89, 1890}, // Mithril Alloy
		{98, 1893}, // Oriharukon
		{100, 951}, // Scroll: Enchant Weapon (C)
	}
}

func rewards10Lose() []reward {
	return []reward{
		{50, 4041}, // Mold Hardener
		{80, 952},  // Scroll: Enchant Armor (C)
		{98, 1892}, // Blacksmith's Frame
		{100, 917}, // Necklace of Mermaid
	}
}

func rewards100Win() []reward {
	return []reward{
		{50, 883},  // Aquastone Ring
		{80, 951},  // Scroll: Enchant Weapon (C)
		{98, 852},  // Moonstone Earring
		{100, 401}, // Drake Leather Armor
	}
}

func rewards100Lose() []reward {
	return []reward{
		{50, 951},  // Scroll: Enchant Weapon (C)
		{80, 500},  // Great Helmet
		{98, 2437}, // Drake Leather Boots
		{100, 135}, // Samurai Longsword
	}
}

// New returns the quest.
func New() script.Script {
	return script.Script{
		Title:   "Warehouse Keeper's Pastime",
		QuestID: 384,
		Bind: script.Bindings{
			script.EventQuestStart: {cliff},
			script.EventTalked:     {cliff, baxt},
			script.EventMyDying: {
				20947, 20948, 20945, 20946, 20635, 20773, 20774, 20760, 20758, 20759, 20242, 20281, 20556,
				20668, 20241, 20286, 20949, 20950, 20942, 20943, 20944, 20559, 20243, 20282, 20677, 20605,
			},
		},
		Hooks: script.Hooks{OnEvent: onAdvEvent, OnTalk: onTalk, OnMyDying: onMyDying},
	}
}

// variable returns the state's variable key; a variable that was never set
// aborts the invocation, as the board does of a pick made before the bet.
func variable(st *script.QuestState, key string) string {
	v, ok := st.Get(key)
	if !ok {
		panic("q384: no variable " + key)
	}
	return v
}

func onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	st := s.QuestState(e.Player, questName)
	if st == nil {
		return htmltext
	}
	p := e.Player
	npcID := strconv.Itoa(int(e.NPC.NpcID()))
	switch {
	case strings.EqualFold(e.Name, "30182-05.htm"):
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		s.PlaySound(p, script.SoundAccept)
	case strings.EqualFold(e.Name, npcID+"-08.htm"):
		s.PlaySound(p, script.SoundGiveUp)
		st.Exit(true)
	case strings.EqualFold(e.Name, npcID+"-11.htm"):
		if p.ItemCount(medal) < 10 {
			htmltext = npcID + "-12.htm"
		} else {
			st.Set("bet", strconv.Itoa(10))
			st.Set("board", s.Scramble("123456789"))
			s.TakeItems(p, medal, 10)
		}
	case strings.EqualFold(e.Name, npcID+"-13.htm"):
		if p.ItemCount(medal) < 100 {
			htmltext = npcID + "-12.htm"
		} else {
			st.Set("bet", strconv.Itoa(100))
			st.Set("board", s.Scramble("123456789"))
			s.TakeItems(p, medal, 100)
		}
	case strings.HasPrefix(e.Name, "select_1-"): // first pick
		// Register the first char.
		st.Set("playerArray", e.Name[9:])

		// Send back the finalized HTM with dynamic content.
		htmltext = fillBoard(st, s.HTMLText(npcID+"-14.htm"))
	case strings.HasPrefix(e.Name, "select_2-"): // pick #2-5
		// Stores the current event for future use.
		number := e.Name[9:]

		// Restore the player array.
		playerArray := variable(st, "playerArray")

		// Verify if the given number is already on the player array, if yes, it's invalid, otherwise register it.
		if strings.Contains(playerArray, number) {
			htmltext = fillBoard(st, s.HTMLText(npcID+"-"+strconv.Itoa(14+2*len(playerArray))+".htm"))
		} else {
			// Stores the final String.
			st.Set("playerArray", playerArray+number)

			htmltext = fillBoard(st, s.HTMLText(npcID+"-"+strconv.Itoa(13+2*len(playerArray))+".htm"))
		}
	case strings.HasPrefix(e.Name, "select_3-"): // pick #6
		// Stores the current event for future use.
		number := e.Name[9:]

		// Restore the player array.
		playerArray := variable(st, "playerArray")

		// Verify if the given number is already on the player array, if yes, it's invalid, otherwise calculate reward.
		if strings.Contains(playerArray, number) {
			htmltext = fillBoard(st, s.HTMLText(npcID+"-26.htm"))
		} else {
			// No need to store the String on player db, but still need to update it.
			playerChoice := playerArray + number

			// Transform the generated board (9 string length) into a 2d matrice (3x3 int).
			board := strings.Split(variable(st, "board"), "")

			// test for all line combination
			winningLines := 0

			lines := [8][3]int{
				{1, 2, 3}, // line 1
				{4, 5, 6}, // line 2
				{7, 8, 9}, // line 3
				{1, 4, 7}, // column 1
				{2, 5, 8}, // column 2
				{3, 6, 9}, // column 3
				{1, 5, 9}, // diagonal 1
				{3, 5, 7}, // diagonal 2
			}
			for _, m := range lines {
				// test line combination
				won := true
				for _, index := range m {
					won = won && strings.Contains(playerChoice, board[index-1])
				}

				// cut the loop, when you won
				if won {
					winningLines++
				}
			}

			switch winningLines {
			case 3:
				htmltext = s.HTMLText(npcID + "-23.htm")

				chance := s.Rnd(100)
				table := rewards100Win()
				if st.GetInt("bet") == 10 {
					table = rewards10Win()
				}
				for _, r := range table {
					if chance < r.chance {
						s.GiveItems(p, r.item, 1)
						if r.item == 2437 {
							s.GiveItems(p, 2463, 1)
						}
						break
					}
				}
			case 0:
				htmltext = s.HTMLText(npcID + "-25.htm")

				chance := s.Rnd(100)
				table := rewards100Lose()
				if st.GetInt("bet") == 10 {
					table = rewards10Lose()
				}
				for _, r := range table {
					if chance < r.chance {
						s.GiveItems(p, r.item, 1)
						break
					}
				}
			default:
				htmltext = s.HTMLText(npcID + "-24.htm")
			}

			for i := 1; i < 10; i++ {
				htmltext = strings.ReplaceAll(htmltext, "<?Cell"+strconv.Itoa(i)+"?>", board[i-1])
				color := "ffffff"
				if strings.Contains(playerChoice, board[i-1]) {
					color = "ff0000"
				}
				htmltext = strings.ReplaceAll(htmltext, "<?FontColor"+strconv.Itoa(i)+"?>", color)
			}
		}
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
		if p.Level() < 40 {
			htmltext = "30182-04.htm"
		} else {
			htmltext = "30182-01.htm"
		}
	case questlog.StatusStarted:
		switch e.NPC.NpcID() {
		case cliff:
			if p.ItemCount(medal) < 10 {
				htmltext = "30182-06.htm"
			} else {
				htmltext = "30182-07.htm"
			}
		case baxt:
			if p.ItemCount(medal) < 10 {
				htmltext = "30685-01.htm"
			} else {
				htmltext = "30685-02.htm"
			}
		}
	}
	return htmltext
}

func onMyDying(s *script.Script, e script.MyDying) {
	st := s.RandomPartyMemberState(script.ActingPlayer(e.Killer), e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	s.DropItems(st.Player(), medal, 1, 0, chances()[e.NPC.NpcID()])
}

// fillBoard fills the cells of the page with the picked numbers of the
// board and a question mark for the cells not picked yet.
func fillBoard(st *script.QuestState, htmltext string) string {
	playerArray := variable(st, "playerArray")
	board := strings.Split(variable(st, "board"), "")

	for i := 1; i < 10; i++ {
		cell := "?"
		if strings.Contains(playerArray, board[i-1]) {
			cell = board[i-1]
		}
		htmltext = strings.ReplaceAll(htmltext, "<?Cell"+strconv.Itoa(i)+"?>", cell)
	}
	return htmltext
}
