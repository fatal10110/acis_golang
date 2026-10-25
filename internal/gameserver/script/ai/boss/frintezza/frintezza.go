package frintezza

import (
	"strconv"
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// frintezzaGMID is the global memo key that holds Frintezza's object id.
const frintezzaGMID = 4

// Frintezza's songs, all cast on himself.
func music() skill.Ref          { return skill.Ref{ID: 5006, Level: 1} }
func musicOfHeal() skill.Ref    { return skill.Ref{ID: 5007, Level: 1} }
func musicOfRampage() skill.Ref { return skill.Ref{ID: 5007, Level: 2} }
func musicOfPower() skill.Ref   { return skill.Ref{ID: 5007, Level: 3} }
func musicOfPlague() skill.Ref  { return skill.Ref{ID: 5007, Level: 4} }
func musicOfPsycho() skill.Ref  { return skill.Ref{ID: 5007, Level: 5} }

// Frintezza (29045), the tomb's master. His db value is the tomb's stage:
// 0 sealed, 1 and 2 entered, 3 awakening, 4 to 7 the fight, his follower's
// two forms (5, 6) and its transform (7) reporting their stages to him by
// script events that carry their object ids as event ids. Global memo key 4
// names him; keys 5 and 6 name the follower and its transform.
//
// Spawned with no row he seals the tomb at db value 0: maker event 1001 on
// frintessa_2515_m01, grave pathways 1 and 2 and every wall closed, 3 and 4
// open, and the follower and its transform told 0. Restored at 1 he runs
// maker event 1001 there again; at 2 he runs 1000 and jumps to his hall 10
// minutes later; at 3 to 7 he closes every door, leaves his organ dummy,
// plays the stage's song and runs maker event 1000.
//
// Script event arg1 1 and 2 (from the teleporter) set the stage and tell the
// follower; 2 also runs maker event 1000 and, FrintezzaWaitTime later, wakes
// him and the follower (arg1 3). Arg1 3 starts his awakening: his organ
// dummy, his jump to the hall facing 16384 with every door closed, two
// socials, both evil spirit makers' event 1001, and his songs; 13.5 s after
// the first song he wakes the follower (arg1 4). Arg1 0, with the follower
// and its transform both named, seals the tomb again and sends him home;
// arg1 8 kills him 7.5 s later; arg1 50000 plays his last song. Every
// finished song plays the next for the stage; an aborted one, or a hit with
// skill 2234, silences him for 30 s. Dying he gives up key 4, seals the tomb
// and runs maker event 1001 on frintessa_teleportcuve.
func Frintezza() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{29045}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			n := e.NPC
			ai1 := n.Int(script.SlotIntAI1)
			c0 := s.GlobalMemo().Creature(strconv.Itoa(frintezzaGMID))
			if c0 == nil {
				s.GlobalMemo().Set(strconv.Itoa(frintezzaGMID), int(n.ObjectID()))
			}
			switch {
			case !n.DBLoaded():
				n.SetDBValue(0)
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1001", 0, 0)
				setTombDoors(s, 0)
				if c1 := memoNPC(s, "5"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
				if c1 := memoNPC(s, "6"); c1 != nil {
					s.SendScriptEvent(c1, n.ObjectID(), 0, 0)
				}
			case n.DBValue() == 1:
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1001", 0, 0)
			case n.DBValue() == 2:
				s.StartTimer("1001", n, nil, 600000*time.Millisecond)
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1000", 0, 0)
			case n.DBValue() > 2 && n.DBValue() < 8:
				setTombDoors(s, 1)
				s.Spawner().CreateOnePrivateEx(n, 29052, script.Loc{X: 174239, Y: -89809, Z: -5016, Heading: 16384}, 0, script.Params{})
				callSong(s, n, ai1)
				s.StartTimer("3000", n, nil, 1000*time.Millisecond)
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1000", 0, 0)
			}
			for _, slot := range []npc.IntSlot{script.SlotIntAI0, script.SlotIntAI1, script.SlotIntAI2, script.SlotIntAI3, script.SlotIntAI4, npc.IntQuest0, npc.IntQuest1, npc.IntQuest2, npc.IntQuest3} {
				n.SetInt(slot, 0)
			}
			base.Created(s, e)
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			n := e.NPC
			ai1 := n.Int(script.SlotIntAI1)
			switch {
			case strings.EqualFold(e.Name, "1000"):
				s.Spawner().CreateOnePrivateEx(n, 29052, script.Loc{X: 174239, Y: -89809, Z: -5016, Heading: 16384}, 0, script.Params{})
				s.StartTimer("1001", n, nil, 4000*time.Millisecond)
			case strings.EqualFold(e.Name, "1001"):
				n.SetHeading(16384)
				n.TeleportTo(script.Loc{X: 174239, Y: -89808, Z: -5016})
				setTombDoors(s, 1)
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				s.StartTimer("1002", n, nil, 6000*time.Millisecond)
			case strings.EqualFold(e.Name, "1002"):
				s.StartTimer("1003", n, nil, 2000*time.Millisecond)
			case strings.EqualFold(e.Name, "1003"):
				n.AddSocialDesire(1, 7000, 1000)
				s.StartTimer("1004", n, nil, 7000*time.Millisecond)
			case strings.EqualFold(e.Name, "1004"):
				s.StartTimer("1005", n, nil, 1500*time.Millisecond)
			case strings.EqualFold(e.Name, "1005"):
				s.Spawner().SendMakerEvent("frintessa_evilate_maker1", "1001", 0, 0)
				s.StartTimer("1006", n, nil, 500*time.Millisecond)
			case strings.EqualFold(e.Name, "1006"):
				s.Spawner().SendMakerEvent("frintessa_evilate_maker2", "1001", 0, 0)
				s.StartTimer("1007", n, nil, 500*time.Millisecond)
			case strings.EqualFold(e.Name, "1007"):
				n.AddSocialDesire(3, 6600, 1000)
				s.StartTimer("1008", n, nil, 6600*time.Millisecond)
			case strings.EqualFold(e.Name, "1008"):
				s.StartTimer("1009", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "1009"):
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				callSong(s, n, ai1)
				s.StartTimer("1010", n, nil, 10000*time.Millisecond)
			case strings.EqualFold(e.Name, "1010"):
				s.StartTimer("1011", n, nil, 3500*time.Millisecond)
			case strings.EqualFold(e.Name, "1011"):
				if c0 := memoNPC(s, "5"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 4, 0)
					n.SetDBValue(4)
				}
				s.StartTimer("1012", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "1012"):
				n.SetInt(npc.IntQuest1, s.CurrentTick())
				callSong(s, n, ai1)
				s.StartTimer("1013", n, nil, 3000*time.Millisecond)
			case strings.EqualFold(e.Name, "1500"):
				s.SendScriptEvent(n, n.ObjectID(), 3, 0)
				if c0 := memoNPC(s, "5"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 3, 0)
				}
			case strings.EqualFold(e.Name, "2000"):
				n.RemoveAllDesire()
				n.AddSocialDesire(4, 7000, 10000000)
				s.StartTimer("2001", n, nil, 7000*time.Millisecond)
			case strings.EqualFold(e.Name, "2001"):
				s.StartTimer("2002", n, nil, 1000*time.Millisecond)
			case strings.EqualFold(e.Name, "2002"):
				n.RemoveAllDesire()
				playSong(s, n, 0, 0, 0, 10000, 0)
				s.StartTimer("2003", n, nil, 4000*time.Millisecond)
			case strings.EqualFold(e.Name, "5000"):
				n.AddSocialDesire(3, 100, 50)
			case strings.EqualFold(e.Name, "6000"):
				s.SendScriptEvent(n, n.ObjectID(), 3, 0)
			case strings.EqualFold(e.Name, "7000"):
				n.DoDie(n)
			case strings.EqualFold(e.Name, "8000"):
				n.SetDBValue(0)
				n.SetHeading(16384)
				n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})
				setTombDoors(s, 0)
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1000", 0, 0)
			case strings.EqualFold(e.Name, "3000"), strings.EqualFold(e.Name, "3001"):
				n.SetInt(script.SlotIntAI4, 0)
				fightSong(s, n, ai1)
			}
			n.SetInt(script.SlotIntAI1, 0)
			return base.Timer(s, script.Timer{Name: e.Name, NPC: n})
		},
		OnScriptEvent: func(s *script.Script, e script.ScriptEvent) {
			n := e.NPC
			if c0 := memoNPC(s, "5"); c0 != nil && e.EventID == c0.ObjectID() {
				switch e.Arg1 {
				case 5:
					n.SetDBValue(5)
					n.SetInt(script.SlotIntAI0, 5)
					n.SetInt(script.SlotIntAI1, 0)
				case 30000:
					n.SetInt(script.SlotIntAI1, 30000)
				case 6:
					n.SetDBValue(6)
					n.SetInt(script.SlotIntAI0, 6)
					n.SetInt(script.SlotIntAI1, 0)
				case 30010:
					n.SetInt(script.SlotIntAI1, 30010)
				}
			} else {
				switch e.Arg1 {
				case 7:
					n.SetDBValue(7)
					n.SetInt(script.SlotIntAI0, 7)
					n.SetInt(script.SlotIntAI1, 0)
					n.SetInt(npc.IntQuest2, s.CurrentTick())
					s.StartTimer("5000", n, nil, 1000*time.Millisecond)
				case 30021, 30022, 30023, 30024, 30025:
					n.SetInt(script.SlotIntAI1, e.Arg1)
				}
			}
			switch e.Arg1 {
			case 0:
				if memoNPC(s, "5") == nil || memoNPC(s, "6") == nil {
					return
				}
				setTombDoors(s, 0)
				n.SetDBValue(0)
				n.SetHeading(16384)
				n.TeleportTo(script.Loc{X: -105200, Y: -253104, Z: -15264})
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1001", 0, 0)
			case 1:
				n.SetDBValue(1)
				if c0 := memoNPC(s, "5"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 1, 0)
				}
			case 2:
				n.SetDBValue(2)
				if c0 := memoNPC(s, "5"); c0 != nil {
					s.SendScriptEvent(c0, n.ObjectID(), 2, 0)
				}
				s.Spawner().SendMakerEvent("frintessa_2515_m01", "1000", 0, 0)
				s.StartTimer("1500", n, nil, s.BossWaitTimes().Frintezza)
			case 3:
				n.SetDBValue(3)
				s.StartTimer("1000", n, nil, 4000*time.Millisecond)
			case 8:
				s.StartTimer("7000", n, nil, 7500*time.Millisecond)
			case 50000:
				s.StartTimer("2000", n, nil, 500*time.Millisecond)
			}
			base.ScriptEvent(s, e)
		},
		OnAttacked: func(s *script.Script, e script.Attacked) {
			n := e.NPC
			if e.Skill.ID == 2234 {
				n.SetInt(script.SlotIntAI4, 1)
				n.RemoveAllDesire()
				s.StartTimer("3000", n, nil, 30000*time.Millisecond)
			}
		},
		OnUseSkillFinished: func(s *script.Script, e script.UseSkillFinished) {
			n := e.NPC
			switch {
			case n.Int(script.SlotIntAI4) != 0:
				playSong(s, n, 0, 0, 0, 10000, 0)
				n.SetInt(script.SlotIntAI4, 0)
			case !e.Success:
				n.SetInt(script.SlotIntAI4, 1)
				n.RemoveAllDesire()
				s.StartTimer("3001", n, nil, 30000*time.Millisecond)
			default:
				fightSong(s, n, n.Int(script.SlotIntAI1))
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			s.GlobalMemo().Remove(strconv.Itoa(frintezzaGMID))
			setTombDoors(s, 0)
			n.SetDBValue(0)
			s.Spawner().SendMakerEvent("frintessa_teleportcuve", "1001", 0, 0)
		},
		OnNoDesire: func(s *script.Script, e script.NoDesire) {
			n := e.NPC
			if n.DBValue() < 4 {
				return
			}
			if n.Int(script.SlotIntAI4) == 0 {
				fightSong(s, n, n.Int(script.SlotIntAI1))
			}
		},
	})}
}

// setTombDoors closes grave pathways 1 and 2 and every wall, and opens (0)
// or closes (1) pathways 3 and 4 as inner says, in the tomb's door order.
func setTombDoors(s *script.Script, inner int32) {
	defaultnpc.OpenCloseDoor(s, "grave_pathway_1", 1)
	defaultnpc.OpenCloseDoor(s, "grave_pathway_2", 1)
	defaultnpc.OpenCloseDoor(s, "grave_pathway_3", inner)
	defaultnpc.OpenCloseDoor(s, "grave_pathway_4", inner)
	for _, name := range []string{
		"wall_door_a_center_1", "wall_door_a_center_2", "wall_door_a_center_3", "wall_door_a_center_4",
		"wall_door_a_center_5", "wall_door_a_center_6", "wall_door_a_center_7", "wall_door_a_center_8",
		"wall_door_b_left_1", "wall_door_b_left_2", "wall_door_b_left_3", "wall_door_b_left_4", "wall_door_b_left_5",
		"wall_door_b_right_1", "wall_door_b_right_2", "wall_door_b_right_3", "wall_door_b_right_4", "wall_door_b_right_5",
	} {
		defaultnpc.OpenCloseDoor(s, name, 1)
	}
}

// callSong plays the song of the stage, as Frintezza takes up his songs:
// ai1 is the stage his follower or its transform last reported.
func callSong(s *script.Script, n *script.NPC, ai1 int32) {
	switch n.DBValue() {
	case 3:
		playSong(s, n, 0, 0, 0, 0, 0)
	case 4:
		playSong(s, n, 0, 0, 0, 0, 0)
	case 5:
		playSong(s, n, 2000, 2000, 0, 0, 0)
	case 6:
		switch ai1 {
		case 0:
			playSong(s, n, 1500, 1500, 1000, 2000, 0)
		case 30010:
			playSong(s, n, 1000, 2000, 1000, 2000, 0)
		}
	case 7:
		switch ai1 {
		case 0:
			playSong(s, n, 0, 1500, 625, 2500, 0)
		case 30021:
			playSong(s, n, 1500, 0, 625, 3000, 0)
		case 30022:
			if s.ElapsedTicks(n.Int(npc.IntQuest2)) > 15*60 {
				playSong(s, n, 0, 1500, 0, 3000, 1500)
			} else {
				playSong(s, n, 1500, 0, 1500, 3000, 0)
			}
		case 30023:
			if s.ElapsedTicks(n.Int(npc.IntQuest2)) > 15*60 {
				playSong(s, n, 1500, 0, 625, 3000, 1500)
			} else {
				playSong(s, n, 1500, 0, 1500, 3000, 0)
			}
		case 30024:
			playSong(s, n, 1500, 1500, 0, 3000, 0)
		}
	}
}

// fightSong plays the song of the stage once Frintezza is in the fight,
// where the healing song all but never plays.
func fightSong(s *script.Script, n *script.NPC, ai1 int32) {
	switch n.DBValue() {
	case 3:
		playSong(s, n, 0, 0, 0, 0, 0)
	case 4:
		playSong(s, n, 0, 0, 0, 0, 0)
	case 5:
		playSong(s, n, 2, 2000, 0, 0, 0)
	case 6:
		switch ai1 {
		case 0:
			playSong(s, n, 1, 1500, 1000, 2000, 0)
		case 30010:
			playSong(s, n, 1, 2000, 1000, 2000, 0)
		}
	case 7:
		switch ai1 {
		case 0:
			playSong(s, n, 0, 1500, 625, 2500, 0)
		case 30021:
			playSong(s, n, 1, 0, 625, 3000, 0)
		case 30022:
			if s.ElapsedTicks(n.Int(npc.IntQuest2)) > 15*60 {
				playSong(s, n, 0, 1500, 0, 3000, 1500)
			} else {
				playSong(s, n, 1, 0, 1500, 3000, 0)
			}
		case 30023:
			if s.ElapsedTicks(n.Int(npc.IntQuest2)) > 15*60 {
				playSong(s, n, 1, 0, 625, 3000, 1500)
			} else {
				playSong(s, n, 1, 0, 1500, 3000, 0)
			}
		case 30024:
			playSong(s, n, 1, 1500, 0, 3000, 0)
		}
	}
}

// playSong wants n to cast one of its songs on itself and shows the song's
// name on the screens around it for 3 s: healing, power, psycho, rampage or
// plague, each at its odds out of 10000 drawn in that order, else the plain
// music.
func playSong(s *script.Script, n *script.NPC, heal, power, psycho, rampage, plague int) {
	switch {
	case s.Rnd(10000) < heal:
		n.AddCastDesireUnchecked(n, musicOfHeal(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000522))
	case s.Rnd(10000) < power:
		n.AddCastDesireUnchecked(n, musicOfPower(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000523))
	case s.Rnd(10000) < psycho:
		n.AddCastDesireUnchecked(n, musicOfPsycho(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000525))
	case s.Rnd(10000) < rampage:
		n.AddCastDesireUnchecked(n, musicOfRampage(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000524))
	case s.Rnd(10000) < plague:
		n.AddCastDesireUnchecked(n, musicOfPlague(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000526))
	default:
		n.AddCastDesireUnchecked(n, music(), 10000)
		n.BroadcastOnScreen(3000, script.NpcString(1000527))
	}
}
