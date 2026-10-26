package frintezza

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
)

// AlarmDevice (18328), the hall alarm. Appearing it clears its ai slot 0
// and shouts 1010630; script event 10025 sets the slot to arg1, the hall
// it guards. Dying with the slot at 1, 2 or 3 it draws against 10, 20 or
// 30 in 100: a hit, or a slot of 4, de-activates the alarm, shouting
// 1010631, running maker event 1000 on the four alarm makers, then on the
// captain, guard, patrol, five wizard and self-destruct makers, and
// opening grave pathways 1 and 2 and the eight center walls; a miss runs
// maker event 1001 on the slot's captain, self-destruct or guard maker.
// The draw is made whether or not that maker exists (#4278).
func AlarmDevice() script.Script {
	base := defaultnpc.New().Hooks
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{18328}, Hooks: base.With(script.Hooks{
		OnCreated: func(_ *script.Script, e script.Created) {
			e.NPC.SetInt(script.SlotIntAI0, 0)
			e.NPC.Shout(script.NpcString(1010630))
		},
		OnScriptEvent: func(_ *script.Script, e script.ScriptEvent) {
			if e.EventID == 10025 {
				e.NPC.SetInt(script.SlotIntAI0, e.Arg1)
			}
		},
		OnMyDying: func(s *script.Script, e script.MyDying) {
			n := e.NPC
			i0 := 0
			switch n.Int(script.SlotIntAI0) {
			case 1:
				i0 = alarmDraw(s, "godard32_2515_07m1", 10)
			case 2:
				i0 = alarmDraw(s, "godard32_2515_09m1", 20)
			case 3:
				i0 = alarmDraw(s, "godard32_2515_06m1", 30)
			case 4:
				i0 = 100
			}
			if i0 != 100 {
				return
			}
			n.Shout(script.NpcString(1010631))
			for _, maker := range []string{
				"godard32_2515_22m1", "godard32_2515_19m1", "godard32_2515_20m1", "godard32_2515_21m1",
				"godard32_2515_07m1", "godard32_2515_06m1", "godard32_2515_05m1",
				"godard32_2515_01m1", "godard32_2515_02m1", "godard32_2515_03m1", "godard32_2515_04m1", "godard32_2515_08m1",
				"godard32_2515_09m1",
			} {
				s.Spawner().SendMakerEvent(maker, "1000", 0, 0)
			}
			for _, door := range []string{
				"grave_pathway_1", "grave_pathway_2",
				"wall_door_a_center_1", "wall_door_a_center_2", "wall_door_a_center_3", "wall_door_a_center_4",
				"wall_door_a_center_5", "wall_door_a_center_6", "wall_door_a_center_7", "wall_door_a_center_8",
			} {
				defaultnpc.OpenCloseDoor(s, door, 0)
			}
		},
	})}
}

// alarmDraw draws against odds in 100: a hit returns 100, a miss runs maker
// event 1001 on maker and returns 0.
func alarmDraw(s *script.Script, maker string, odds int) int {
	if s.Rnd(100) < odds {
		return 100
	}
	s.Spawner().SendMakerEvent(maker, "1001", 0, 0)
	return 0
}
