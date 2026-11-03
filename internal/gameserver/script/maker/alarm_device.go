package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The four alarm device makers of the Imperial Tomb are one device each.
// They spawn nothing at start. A device's NPCs hear script event 10025
// carrying the device's count, which starts at 1 and grows by one with
// each device lost: the last NPC of a device leaving raises 10008 on that
// device and on the other three devices' npcmakers, by name. Their NPCs
// never respawn on their own.

// AlarmDevice1 is the alarm device whose loss raises 10008 on the
// godard32_2515_19m1, 20m1 and 21m1 devices.
func AlarmDevice1() script.Maker {
	count := &intMemo{key: "i_ai0"}
	return script.Maker{MakerHooks: alarmDevice1Hooks(count)}
}

// alarmDevice1Hooks are AlarmDevice1's hooks over its count memo (i_ai0),
// the parent of AlarmDevice2 and AlarmDevice3. Start sets the count to 1
// and, three seconds later, timer 2000 sends the first spawn's NPCs 10025
// with 1. Script event 10008 adds one to the count and sends the first
// spawn's NPCs 10025 with it; 1001 sets the count back to 1 and arms timer
// 2000 again. Both then run the default maker's script events.
func alarmDevice1Hooks(count *intMemo) script.MakerHooks {
	base := defaultHooks()
	return base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			count.set(1)
			armAlarmTimer(m, e.Group)
		},
		OnTimer: func(_ *script.Maker, e script.MakerTimer) {
			alarmTimer(e)
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			alarmScriptEvent(m, e, count)
			base.ScriptEvent(m, e)
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			alarmDeviceLost(m, e.Group, count, false, "godard32_2515_19m1", "godard32_2515_20m1", "godard32_2515_21m1")
		},
	})
}

// AlarmDevice2 is AlarmDevice1 whose loss also sends its NPCs 10025 once
// more and raises 10008 on the godard32_2515_22m1, 20m1 and 21m1 devices.
func AlarmDevice2() script.Maker {
	count := &intMemo{key: "i_ai0"}
	base := alarmDevice1Hooks(count)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			alarmDeviceLost(m, e.Group, count, true, "godard32_2515_22m1", "godard32_2515_20m1", "godard32_2515_21m1")
		},
	})}
}

// AlarmDevice3 is AlarmDevice1 whose loss also sends its NPCs 10025 once
// more and raises 10008 on the godard32_2515_19m1, 22m1 and 21m1 devices.
func AlarmDevice3() script.Maker {
	count := &intMemo{key: "i_ai0"}
	base := alarmDevice1Hooks(count)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			alarmDeviceLost(m, e.Group, count, true, "godard32_2515_19m1", "godard32_2515_22m1", "godard32_2515_21m1")
		},
	})}
}

// AlarmDevice4 is a device of its own with AlarmDevice1's start, timer
// and script events, whose loss raises 10008 on the godard32_2515_19m1,
// 20m1 and 22m1 devices.
func AlarmDevice4() script.Maker {
	count := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			count.set(1)
			armAlarmTimer(m, e.Group)
		},
		OnTimer: func(_ *script.Maker, e script.MakerTimer) {
			alarmTimer(e)
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			alarmScriptEvent(m, e, count)
			base.ScriptEvent(m, e)
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			alarmDeviceLost(m, e.Group, count, false, "godard32_2515_19m1", "godard32_2515_20m1", "godard32_2515_22m1")
		},
	})}
}

// armAlarmTimer runs the maker's timer 2000 for g three seconds from now.
func armAlarmTimer(m *script.Maker, g spawn.Group) {
	g.After(3000*time.Millisecond, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "2000"}) })
}

// alarmTimer answers timer 2000: the first spawn's NPCs hear 10025 with 1.
func alarmTimer(e script.MakerTimer) {
	if !strings.EqualFold(e.Name, "2000") {
		return
	}
	if def0 := firstSpawn(e.Group); def0 != nil {
		def0.SendScriptEvent(10025, 1, 0)
	}
}

// alarmScriptEvent answers a device's own script events ahead of the
// default maker's: 10008 adds one to the count and sends the first spawn's
// NPCs 10025 with it; 1001 sets the count back to 1 and arms timer 2000.
func alarmScriptEvent(m *script.Maker, e script.MakerEvent, count *intMemo) {
	switch {
	case strings.EqualFold(e.Name, "10008"):
		count.set(count.get() + 1)
		if def0 := firstSpawn(e.Group); def0 != nil {
			def0.SendScriptEvent(10025, int32(count.get()), 0)
		}
	case strings.EqualFold(e.Name, "1001"):
		count.set(1)
		armAlarmTimer(m, e.Group)
	}
}

// alarmDeviceLost runs when an NPC of a device has left: once the group's
// last NPC is gone, the maker runs its own script event 10008, sends the
// first spawn's NPCs 10025 with the count again when resend is set, runs
// 10008 on each of the named npcmakers in order, and sets its count to 0.
func alarmDeviceLost(m *script.Maker, g spawn.Group, count *intMemo, resend bool, others ...string) {
	if g.Alive() != 0 {
		return
	}
	m.ScriptEvent(m, script.MakerEvent{Group: g, Name: "10008"})
	if resend {
		if def0 := firstSpawn(g); def0 != nil {
			def0.SendScriptEvent(10025, int32(count.get()), 0)
		}
	}
	for _, name := range others {
		g.SendMakerEvent(name, "10008", 0, 0)
	}
	count.set(0)
}

// firstSpawn is the group's first spawn; nil when it has none.
func firstSpawn(g spawn.Group) spawn.GroupSpawn {
	if s := g.Spawns(); len(s) > 0 {
		return s[0]
	}
	return nil
}
