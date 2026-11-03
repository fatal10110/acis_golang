// Package q230 is the quest Test of the Summoner, a second class-change
// trial: Galatea sends a wizard to Lara for the beginner's arcana, then to
// the six arcana masters, whose monsters the wizard's summon must beat in a
// fair duel, and hands over the Mark of Summoner.
package q230

import (
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/questlog"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/secondclass"
)

const questName = "Q230_TestOfTheSummoner"

// Items.
const (
	letoLizardmanAmulet = 3337
	sacOfRedspores      = 3338
	karulBugbearTotem   = 3339
	shardsOfManashen    = 3340
	brekaOrcTotem       = 3341
	crimsonBloodstone   = 3342
	talonsOfTyrant      = 3343
	wingsOfDroneant     = 3344
	tuskOfWindsus       = 3345
	fangsOfWyrm         = 3346
	laraList1           = 3347
	laraList2           = 3348
	laraList3           = 3349
	laraList4           = 3350
	laraList5           = 3351
	galateaLetter       = 3352
	beginnerArcana      = 3353
	almorsArcana        = 3354
	camoniellArcana     = 3355
	belthusArcana       = 3356
	basilliaArcana      = 3357
	celestielArcana     = 3358
	bryntheaArcana      = 3359
)

// The crystals of the six duels: progress, in progress, foul, defeat and
// victory, per duel.
const (
	crystalOfProgress1   = 3360
	crystalOfInProgress1 = 3361
	crystalOfFoul1       = 3362
	crystalOfDefeat1     = 3363
	crystalOfVictory1    = 3364
	crystalOfProgress2   = 3365
	crystalOfInProgress2 = 3366
	crystalOfFoul2       = 3367
	crystalOfDefeat2     = 3368
	crystalOfVictory2    = 3369
	crystalOfProgress3   = 3370
	crystalOfInProgress3 = 3371
	crystalOfFoul3       = 3372
	crystalOfDefeat3     = 3373
	crystalOfVictory3    = 3374
	crystalOfProgress4   = 3375
	crystalOfInProgress4 = 3376
	crystalOfFoul4       = 3377
	crystalOfDefeat4     = 3378
	crystalOfVictory4    = 3379
	crystalOfProgress5   = 3380
	crystalOfInProgress5 = 3381
	crystalOfFoul5       = 3382
	crystalOfDefeat5     = 3383
	crystalOfVictory5    = 3384
	crystalOfProgress6   = 3385
	crystalOfInProgress6 = 3386
	crystalOfFoul6       = 3387
	crystalOfDefeat6     = 3388
	crystalOfVictory6    = 3389
)

// markOfSummoner is the reward.
const markOfSummoner = 3336

// NPCs.
const (
	lara      = 30063
	galatea   = 30634
	almors    = 30635
	camoniell = 30636
	belthus   = 30637
	basilla   = 30638
	celestiel = 30639
	brynthea  = 30640
)

// Monsters.
const (
	nobleAnt              = 20089
	nobleAntLeader        = 20090
	wyrm                  = 20176
	tyrant                = 20192
	tyrantKingpin         = 20193
	brekaOrc              = 20267
	brekaOrcArcher        = 20268
	brekaOrcShaman        = 20269
	brekaOrcOverlord      = 20270
	brekaOrcWarrior       = 20271
	fetteredSoul          = 20552
	windsus               = 20553
	giantFungus           = 20555
	manashenGargoyle      = 20563
	letoLizardman         = 20577
	letoLizardmanArcher   = 20578
	letoLizardmanSoldier  = 20579
	letoLizardmanWarrior  = 20580
	letoLizardmanShaman   = 20581
	letoLizardmanOverlord = 20582
	karulBugbear          = 20600
)

// The duel monsters.
const (
	pakoTheCat      = 27102
	unicornRacer    = 27103
	shadowTuren     = 27104
	mimiTheCat      = 27105
	unicornPhantasm = 27106
	silhouetteTilfo = 27107
)

// The classes the trial is open to.
const (
	classWizard      = 11
	classElvenWizard = 26
	classDarkWizard  = 39
)

// duelBuff is the skill an arcana master casts on the player as a duel
// starts, at level 1, and duelBuffWeight the weight of that cast desire.
const (
	duelBuff       = 4126
	duelBuffWeight = 1000000
)

// The states of an arcana master's duel, kept in the master's variable.
const (
	duelOffered    = 1
	duelReady      = 2
	duelInProgress = 3
	duelLost       = 4
	duelFouled     = 5
	duelWon        = 6
	duelDone       = 7
)

// laraLists returns the five lists Lara hands out: the list, then the two
// items it asks thirty of each.
func laraLists() [5][3]int32 {
	return [5][3]int32{
		{laraList1, sacOfRedspores, letoLizardmanAmulet},
		{laraList2, karulBugbearTotem, shardsOfManashen},
		{laraList3, crimsonBloodstone, brekaOrcTotem},
		{laraList4, tuskOfWindsus, talonsOfTyrant},
		{laraList5, wingsOfDroneant, fangsOfWyrm},
	}
}

// master is one arcana master and the duel its monster fights.
type master struct {
	npc     int32
	monster int32
	// memo is the quest variable holding the duel's state.
	memo string
	// arcana is what the master gives for a won duel.
	arcana int32
	// The duel's crystals.
	progress, inProgress, foul, defeat, victory int32
	// The NpcStrings the monster says as the duel starts, is fouled and is
	// lost.
	startSay, foulSay, loseSay int32
	pages                      masterPages
}

// masterPages are an arcana master's pages.
type masterPages struct {
	// offer is the master's first page; ask the link asking for a duel,
	// askArcana its answer to a player holding the beginner's arcana; fight
	// the link starting the duel.
	offer, ask, askArcana, fight string
	// lost, fouled, won, ready, inDuel and done answer the duel's states.
	lost, fouled, won, ready, inDuel, done string
}

// masters returns the six arcana masters.
func masters() []master {
	return []master{
		{
			npc: almors, monster: pakoTheCat, memo: "Almors", arcana: almorsArcana,
			progress: crystalOfProgress1, inProgress: crystalOfInProgress1, foul: crystalOfFoul1, defeat: crystalOfDefeat1, victory: crystalOfVictory1,
			startSay: 23063, foulSay: 23064, loseSay: 23065,
			pages: masterPages{
				offer: "30635-01.htm", ask: "30635-02.htm", askArcana: "30635-03.htm", fight: "30635-04.htm",
				lost: "30635-05.htm", fouled: "30635-06.htm", won: "30635-07.htm", ready: "30635-08.htm", inDuel: "30635-09.htm", done: "30635-10.htm",
			},
		},
		{
			npc: camoniell, monster: unicornRacer, memo: "Camoniell", arcana: camoniellArcana,
			progress: crystalOfProgress2, inProgress: crystalOfInProgress2, foul: crystalOfFoul2, defeat: crystalOfDefeat2, victory: crystalOfVictory2,
			startSay: 23060, foulSay: 23061, loseSay: 23062,
			pages: masterPages{
				offer: "30636-01.htm", ask: "30636-02.htm", askArcana: "30636-03.htm", fight: "30636-04.htm",
				lost: "30636-05.htm", fouled: "30636-06.htm", won: "30636-07.htm", ready: "30636-08.htm", inDuel: "30636-09.htm", done: "30636-10.htm",
			},
		},
		{
			npc: belthus, monster: shadowTuren, memo: "Belthus", arcana: belthusArcana,
			progress: crystalOfProgress3, inProgress: crystalOfInProgress3, foul: crystalOfFoul3, defeat: crystalOfDefeat3, victory: crystalOfVictory3,
			startSay: 23072, foulSay: 23073, loseSay: 23074,
			pages: masterPages{
				offer: "30637-01.htm", ask: "30637-02.htm", askArcana: "30637-03.htm", fight: "30637-04.htm",
				lost: "30637-05.htm", fouled: "30637-06.htm", won: "30637-07.htm", ready: "30637-08.htm", inDuel: "30637-09.htm", done: "30637-10.htm",
			},
		},
		{
			npc: basilla, monster: mimiTheCat, memo: "Basilla", arcana: basilliaArcana,
			progress: crystalOfProgress4, inProgress: crystalOfInProgress4, foul: crystalOfFoul4, defeat: crystalOfDefeat4, victory: crystalOfVictory4,
			startSay: 23066, foulSay: 23067, loseSay: 23068,
			pages: masterPages{
				offer: "30638-01.htm", ask: "30638-02.htm", askArcana: "30638-03.htm", fight: "30638-04.htm",
				lost: "30638-05.htm", fouled: "30638-06.htm", won: "30638-07.htm", ready: "30638-08.htm", inDuel: "30638-09.htm", done: "30638-10.htm",
			},
		},
		{
			npc: celestiel, monster: unicornPhantasm, memo: "Celestiel", arcana: celestielArcana,
			progress: crystalOfProgress5, inProgress: crystalOfInProgress5, foul: crystalOfFoul5, defeat: crystalOfDefeat5, victory: crystalOfVictory5,
			startSay: 23069, foulSay: 23070, loseSay: 23071,
			pages: masterPages{
				offer: "30639-01.htm", ask: "30639-02.htm", askArcana: "30639-03.htm", fight: "30639-04.htm",
				lost: "30639-05.htm", fouled: "30639-06.htm", won: "30639-07.htm", ready: "30639-08.htm", inDuel: "30639-09.htm", done: "30639-10.htm",
			},
		},
		{
			npc: brynthea, monster: silhouetteTilfo, memo: "Brynthea", arcana: bryntheaArcana,
			progress: crystalOfProgress6, inProgress: crystalOfInProgress6, foul: crystalOfFoul6, defeat: crystalOfDefeat6, victory: crystalOfVictory6,
			startSay: 23075, foulSay: 23076, loseSay: 23077,
			pages: masterPages{
				offer: "30640-01.htm", ask: "30640-02.htm", askArcana: "30640-03.htm", fight: "30640-04.htm",
				lost: "30640-05.htm", fouled: "30640-06.htm", won: "30640-07.htm", ready: "30640-08.htm", inDuel: "30640-09.htm", done: "30640-10.htm",
			},
		},
	}
}

// quest holds what the hooks share: the masters, read only, and the duels
// in progress, keyed by the duel monster's npc id, each holding the player
// whose summon started it.
type quest struct {
	masters []master
	duels   *script.MapCell[int32, *script.Player]
}

// New returns the quest.
func New() script.Script {
	q := quest{masters: masters(), duels: &script.MapCell[int32, *script.Player]{}}
	return script.Script{
		Title:   "Test of the Summoner",
		QuestID: 230,
		Items: []int32{
			letoLizardmanAmulet, sacOfRedspores, karulBugbearTotem, shardsOfManashen, brekaOrcTotem, crimsonBloodstone,
			talonsOfTyrant, wingsOfDroneant, tuskOfWindsus, fangsOfWyrm, laraList1, laraList2, laraList3, laraList4, laraList5,
			galateaLetter, beginnerArcana, almorsArcana, camoniellArcana, belthusArcana, basilliaArcana, celestielArcana, bryntheaArcana,
			crystalOfProgress1, crystalOfInProgress1, crystalOfFoul1, crystalOfDefeat1, crystalOfVictory1,
			crystalOfProgress2, crystalOfInProgress2, crystalOfFoul2, crystalOfDefeat2, crystalOfVictory2,
			crystalOfProgress3, crystalOfInProgress3, crystalOfFoul3, crystalOfDefeat3, crystalOfVictory3,
			crystalOfProgress4, crystalOfInProgress4, crystalOfFoul4, crystalOfDefeat4, crystalOfVictory4,
			crystalOfProgress5, crystalOfInProgress5, crystalOfFoul5, crystalOfDefeat5, crystalOfVictory5,
			crystalOfProgress6, crystalOfInProgress6, crystalOfFoul6, crystalOfDefeat6, crystalOfVictory6,
		},
		Bind: script.Bindings{
			script.EventQuestStart: {galatea},
			script.EventTalked:     {galatea, almors, camoniell, belthus, basilla, celestiel, brynthea, lara},
			script.EventAttacked:   {pakoTheCat, unicornRacer, shadowTuren, mimiTheCat, unicornPhantasm, silhouetteTilfo},
			script.EventMyDying: {
				nobleAnt, nobleAntLeader, wyrm, tyrant, tyrantKingpin, brekaOrc, brekaOrcArcher, brekaOrcShaman, brekaOrcOverlord,
				brekaOrcWarrior, fetteredSoul, windsus, giantFungus, manashenGargoyle, letoLizardman, letoLizardmanArcher,
				letoLizardmanSoldier, letoLizardmanWarrior, letoLizardmanShaman, letoLizardmanOverlord, karulBugbear,
				pakoTheCat, unicornRacer, shadowTuren, mimiTheCat, unicornPhantasm, silhouetteTilfo,
			},
		},
		// The death hook makes the quest one its player's death reaches.
		Hooks: script.Hooks{
			OnEvent: q.onAdvEvent, OnTalk: q.onTalk, OnDeath: q.onDeath,
			OnAttacked: q.onAttacked, OnMyDying: q.onMyDying,
		},
	}
}

// byNPC returns the master whose npc id is npc.
func (q quest) byNPC(npc int32) (master, bool) {
	for _, m := range q.masters {
		if m.npc == npc {
			return m, true
		}
	}
	return master{}, false
}

// byMonster returns the master whose duel monster has npc id monster.
func (q quest) byMonster(monster int32) (master, bool) {
	for _, m := range q.masters {
		if m.monster == monster {
			return m, true
		}
	}
	return master{}, false
}

// otherArcanas returns the arcanas of every master but m.
func (q quest) otherArcanas(m master) []int32 {
	var out []int32
	for _, o := range q.masters {
		if o.npc != m.npc {
			out = append(out, o.arcana)
		}
	}
	return out
}

func (q quest) onAdvEvent(s *script.Script, e script.Event) string {
	htmltext := e.Name
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return ""
	}

	switch e.Name {
	// Galatea.
	case "30634-08.htm":
		st.SetStatus(questlog.StatusStarted)
		st.SetCond(1)
		for _, memo := range []string{"Belthus", "Brynthea", "Celestiel", "Camoniell", "Basilla", "Almors"} {
			st.Set(memo, strconv.Itoa(duelOffered))
		}
		s.PlaySound(p, script.SoundAccept)
		s.GiveItems(p, galateaLetter, 1)
		if secondclass.GiveDimensionalDiamonds39(s, p) {
			htmltext = "30634-08a.htm"
		}
		return htmltext
	// Lara, handing out her first list.
	case "30063-02.htm":
		st.SetCond(2)
		s.PlaySound(p, script.SoundMiddle)
		s.TakeItems(p, galateaLetter, 1)
		random := s.Rnd(5)
		s.GiveItems(p, laraLists()[random][0], 1)
		// One more than the list's index, so no list reads as 0.
		st.Set("Lara", strconv.Itoa(random+1))
		return htmltext
	// Lara, handing out a later list.
	case "30063-04.htm":
		random := s.Rnd(5)
		s.PlaySound(p, script.SoundItemGet)
		s.GiveItems(p, laraLists()[random][0], 1)
		st.Set("Lara", strconv.Itoa(random+1))
		return htmltext
	}

	// The arcana masters.
	for _, m := range q.masters {
		switch e.Name {
		case m.pages.ask:
			if p.HasItem(beginnerArcana) {
				htmltext = m.pages.askArcana
			}
			return htmltext
		case m.pages.fight:
			st.Set(m.memo, strconv.Itoa(duelReady))
			s.PlaySound(p, script.SoundItemGet)
			// In case the player cheated or lost them.
			s.TakeItems(p, m.foul, -1)
			s.TakeItems(p, m.defeat, -1)
			s.TakeItems(p, beginnerArcana, 1)
			s.GiveItems(p, m.progress, 1)
			e.NPC.AddCastDesire(p, skill.Ref{ID: duelBuff, Level: 1}, duelBuffWeight)
			return htmltext
		}
	}
	return htmltext
}

func (q quest) onTalk(s *script.Script, e script.Talk) string {
	htmltext := script.NoQuestMsg()
	p := e.Player
	st := s.QuestState(p, questName)
	if st == nil {
		return htmltext
	}

	switch st.Status() {
	case questlog.StatusCreated:
		switch class := p.ClassID(); {
		case class != classWizard && class != classElvenWizard && class != classDarkWizard:
			htmltext = "30634-01.htm"
		case p.Level() < 39:
			htmltext = "30634-02.htm"
		default:
			htmltext = "30634-03.htm"
		}

	case questlog.StatusStarted:
		cond := st.Cond()
		switch npc := e.NPC.NpcID(); npc {
		case lara:
			switch {
			case cond == 1:
				htmltext = "30063-01.htm"
			case st.GetInt("Lara") == 0:
				// No list taken yet: give one.
				htmltext = "30063-03.htm"
			default:
				list := laraLists()[st.GetInt("Lara")-1]
				if p.ItemCount(list[1]) < 30 || p.ItemCount(list[2]) < 30 {
					htmltext = "30063-05.htm"
				} else {
					htmltext = "30063-06.htm"
					st.SetCond(3)
					st.Unset("Lara")
					s.PlaySound(p, script.SoundMiddle)
					s.TakeItems(p, list[0], 1)
					s.TakeItems(p, list[1], -1)
					s.TakeItems(p, list[2], -1)
					s.GiveItems(p, beginnerArcana, 2)
				}
			}

		case galatea:
			switch {
			case cond == 1:
				htmltext = "30634-09.htm"
			case cond == 2 || cond == 3:
				if !p.HasItem(beginnerArcana) {
					htmltext = "30634-10.htm"
				} else {
					htmltext = "30634-11.htm"
				}
			case cond == 4:
				htmltext = "30634-12.htm"
				for _, id := range []int32{
					beginnerArcana, almorsArcana, basilliaArcana, belthusArcana, bryntheaArcana, camoniellArcana, celestielArcana,
					laraList1, laraList2, laraList3, laraList4, laraList5,
				} {
					s.TakeItems(p, id, -1)
				}
				s.GiveItems(p, markOfSummoner, 1)
				s.RewardExpAndSp(p, 148409, 30000)
				p.SocialAction(3)
				s.PlaySound(p, script.SoundFinish)
				st.Exit(false)
			}

		default:
			if m, ok := q.byNPC(npc); ok {
				htmltext = q.masterTalk(s, st, p, m, htmltext)
			}
		}

	case questlog.StatusCompleted:
		htmltext = script.AlreadyCompletedMsg()
	}
	return htmltext
}

// masterTalk answers a talk to the arcana master m by the state of its
// duel; any other state keeps htmltext.
func (q quest) masterTalk(s *script.Script, st *script.QuestState, p *script.Player, m master, htmltext string) string {
	switch st.GetInt(m.memo) {
	case duelOffered:
		return m.pages.offer
	case duelReady:
		return m.pages.ready
	case duelInProgress:
		return m.pages.inDuel
	case duelLost:
		return m.pages.lost
	case duelFouled:
		return m.pages.fouled
	case duelWon:
		st.Set(m.memo, strconv.Itoa(duelDone))
		s.TakeItems(p, m.victory, -1)
		s.GiveItems(p, m.arcana, 1)
		if p.HasItems(q.otherArcanas(m)...) {
			st.SetCond(4)
			s.PlaySound(p, script.SoundMiddle)
		} else {
			s.PlaySound(p, script.SoundItemGet)
		}
		return m.pages.won
	case duelDone:
		return m.pages.done
	}
	return htmltext
}

// onDeath: a duel monster that kills the player, or the player's summon,
// wins its duel.
func (q quest) onDeath(s *script.Script, e script.Death) {
	killer, ok := e.Killer.(*script.NPC)
	if !ok || !killer.Attackable() {
		return
	}
	p := e.Player
	st := s.CheckPlayerState(p, killer, questlog.StatusStarted)
	if st == nil {
		return
	}
	m, ok := q.byMonster(killer.NpcID())
	if !ok || st.GetInt(m.memo) != duelInProgress {
		return
	}
	st.Set(m.memo, strconv.Itoa(duelLost))
	s.PlaySound(p, script.SoundItemGet)
	s.GiveItems(p, m.defeat, 1)
}

// onAttacked: a summon's first hit on a duel monster at full HP starts the
// duel; a hit by anything but a summon while it runs fouls it.
func (q quest) onAttacked(s *script.Script, e script.Attacked) {
	p := script.ActingPlayer(e.Attacker)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()
	m, ok := q.byMonster(npcID)
	if !ok {
		return
	}
	isPet := e.Attacker.Summon()

	switch state := st.GetInt(m.memo); {
	case state == duelReady && isPet && e.NPC.HPRatio() == 1:
		st.Set(m.memo, strconv.Itoa(duelInProgress))
		s.PlaySound(p, script.SoundItemGet)
		s.TakeItems(p, m.progress, -1)
		s.GiveItems(p, m.inProgress, 1)
		e.NPC.Say(script.NpcString(m.startSay))
		q.duels.Set(npcID, p)
	case state == duelInProgress:
		duelist, running := q.duels.Get(npcID)
		// The duel records the summon of its attacker, a summon, which
		// has no summon of its own: a summon's hit, whoever owns it, never
		// fouls the duel, and anything else's always does.
		if !running || isPet || duelist == nil {
			return
		}
		foulSt := s.QuestState(duelist, questName)
		if foulSt == nil {
			return
		}
		// The foul is written to the duelist's state, while the crystals
		// are taken from and given to the player who fouled.
		foulSt.Set(m.memo, strconv.Itoa(duelFouled))
		s.TakeItems(p, m.progress, -1)
		s.TakeItems(p, m.inProgress, -1)
		s.GiveItems(p, m.foul, 1)
		e.NPC.Say(script.NpcString(m.foulSay))
		e.NPC.DoDie(e.NPC)
	}
}

func (q quest) onMyDying(s *script.Script, e script.MyDying) {
	p := script.ActingPlayer(e.Killer)
	st := s.CheckPlayerState(p, e.NPC, questlog.StatusStarted)
	if st == nil {
		return
	}
	npcID := e.NPC.NpcID()

	switch npcID {
	case giantFungus:
		if st.GetInt("Lara") == 1 {
			s.DropItems(p, sacOfRedspores, 1, 30, 800000)
		}
	case letoLizardman, letoLizardmanArcher:
		if st.GetInt("Lara") == 1 {
			s.DropItems(p, letoLizardmanAmulet, 1, 30, 250000)
		}
	case letoLizardmanSoldier, letoLizardmanWarrior:
		if st.GetInt("Lara") == 1 {
			s.DropItems(p, letoLizardmanAmulet, 1, 30, 500000)
		}
	case letoLizardmanShaman, letoLizardmanOverlord:
		if st.GetInt("Lara") == 1 {
			s.DropItems(p, letoLizardmanAmulet, 1, 30, 750000)
		}
	case manashenGargoyle:
		if st.GetInt("Lara") == 2 {
			s.DropItems(p, shardsOfManashen, 1, 30, 800000)
		}
	case karulBugbear:
		if st.GetInt("Lara") == 2 {
			s.DropItems(p, karulBugbearTotem, 1, 30, 800000)
		}
	case brekaOrc, brekaOrcArcher, brekaOrcWarrior:
		if st.GetInt("Lara") == 3 {
			s.DropItems(p, brekaOrcTotem, 1, 30, 250000)
		}
	case brekaOrcShaman, brekaOrcOverlord:
		if st.GetInt("Lara") == 3 {
			s.DropItems(p, brekaOrcTotem, 1, 30, 500000)
		}
	case fetteredSoul:
		if st.GetInt("Lara") == 3 {
			s.DropItems(p, crimsonBloodstone, 1, 30, 600000)
		}
	case windsus:
		if st.GetInt("Lara") == 4 {
			s.DropItems(p, tuskOfWindsus, 1, 30, 700000)
		}
	case tyrant, tyrantKingpin:
		if st.GetInt("Lara") == 4 {
			s.DropItems(p, talonsOfTyrant, 1, 30, 500000)
		}
	case nobleAnt, nobleAntLeader:
		if st.GetInt("Lara") == 5 {
			s.DropItems(p, wingsOfDroneant, 1, 30, 600000)
		}
	case wyrm:
		if st.GetInt("Lara") == 5 {
			s.DropItems(p, fangsOfWyrm, 1, 30, 500000)
		}
	default:
		m, ok := q.byMonster(npcID)
		if !ok {
			return
		}
		if _, running := q.duels.Get(npcID); st.GetInt(m.memo) == duelInProgress && running {
			st.Set(m.memo, strconv.Itoa(duelWon))
			s.PlaySound(p, script.SoundMiddle)
			s.TakeItems(p, m.inProgress, -1)
			s.GiveItems(p, m.victory, 1)
			e.NPC.Say(script.NpcString(m.loseSay))
			q.duels.Delete(npcID)
		}
	}
}
