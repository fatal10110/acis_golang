package hall

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The hot springs game managers of the four circles.
const (
	circle1ManagerID = 35596
	circle2ManagerID = 35597
	circle3ManagerID = 35598
	circle4ManagerID = 35599
)

// reportGourd has the manager n, while it has reported no gourd, report
// the gourd it sees, if seen is one: it keeps its object id and tells the
// NPCs within 8000 through script event eventID.
func reportGourd(s *script.Script, n *script.NPC, seen script.Creature, eventID int32) {
	if c, ok := seen.(*script.NPC); ok && n.Int(npc.IntAI3) == 0 {
		if id := c.NpcID(); id >= 35588 && id <= 35591 {
			n.SetInt(npc.IntAI3, seen.ObjectID())
			s.BroadcastScriptEvent(n, eventID, n.Int(npc.IntAI3), 8000)
		}
	}
}

// circle1Hooks are AzitWateringGameManagerCircle1's hooks, which the other
// circles derive their own from. Building them makes the managers run as
// circle 1 with the word game.
func circle1Hooks(w *WateringGame) script.Hooks {
	base := GameManager(w).Hooks.Named("AzitWateringGameManager")
	w.gameMaster.Set(1)
	w.circle.Set(1)
	return base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.SetInt(npc.IntAI3, 0)
			base.Created(s, e)
		},
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			reportGourd(s, e.NPC, e.Creature, 10002)
			base.SeeCreature(s, e)
		},
	}).Named("AzitWateringGameManagerCircle1")
}

// Circle1 is AzitWateringGameManagerCircle1: the hot springs game manager
// of the first circle, which reports the first gourd it sees to the NPCs
// around through script event 10002. Building it makes the managers run
// as circle 1 with the word game. w is the managers' shared values.
func Circle1(w *WateringGame) script.Script {
	return script.Script{
		Dir: "ai/siegeablehall/AzitWateringGameManager", Behavior: true, NPCs: []int32{circle1ManagerID},
		Bind:  script.Bindings{script.EventTalked: {circle1ManagerID}},
		Hooks: circle1Hooks(w),
	}
}

// circleManager is the manager id of a circle after the first, with
// hooks: the first circle's manager talks through it too.
func circleManager(id int32, hooks script.Hooks) script.Script {
	return script.Script{
		Dir: "ai/siegeablehall/AzitWateringGameManager", Behavior: true, NPCs: []int32{id},
		Bind: script.Bindings{
			script.EventFirstTalk: {circle1ManagerID, id},
			script.EventTalked:    {circle1ManagerID, id},
		},
		Hooks: hooks,
	}
}

// Circle2 is AzitWateringGameManagerCircle2: the manager of the second
// circle, reporting its gourd through script event 10003. Building it
// makes the managers run as circle 2 with the word game.
func Circle2(w *WateringGame) script.Script {
	base := circle1Hooks(w)
	w.circle.Set(2)
	return circleManager(circle2ManagerID, base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			reportGourd(s, e.NPC, e.Creature, 10003)
			base.SeeCreature(s, e)
		},
	}))
}

// Circle3 is AzitWateringGameManagerCircle3: the manager of the third
// circle, reporting its gourd through script event 10004. Building it
// makes the managers run as circle 3 with the word game.
func Circle3(w *WateringGame) script.Script {
	base := circle1Hooks(w)
	w.circle.Set(3)
	return circleManager(circle3ManagerID, base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			reportGourd(s, e.NPC, e.Creature, 10004)
			base.SeeCreature(s, e)
		},
	}))
}

// Circle4 is AzitWateringGameManagerCircle4: the manager of the fourth
// circle, reporting its gourd through script event 10005. Building it
// makes the managers run as circle 4 with the word game.
func Circle4(w *WateringGame) script.Script {
	base := circle1Hooks(w)
	w.circle.Set(4)
	return circleManager(circle4ManagerID, base.With(script.Hooks{
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) {
			reportGourd(s, e.NPC, e.Creature, 10005)
			base.SeeCreature(s, e)
		},
	}))
}
