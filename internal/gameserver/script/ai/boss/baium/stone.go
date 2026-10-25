package baium

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// slotWoken is the stone's script memory: 1 once a talk woke Baium.
const slotWoken = script.SlotIntAI0

// noMemo is what a read of a memo key that holds nothing answers: no stored
// value, all of which are 32-bit ints.
const noMemo = math.MinInt32 - 1

// BaiumStone is the stone Baium sleeps as in its lair. The first talk to it
// wakes Baium with script event 10025 from the talker, and the stone leaves
// the world; any later talk does nothing.
func BaiumStone() script.Script {
	return script.Script{
		Dir: dir,
		Bind: script.Bindings{
			script.EventCreated: {stoneID},
			script.EventTalked:  {stoneID},
		},
		Hooks: script.Hooks{
			OnCreated: func(_ *script.Script, e script.Created) {
				e.NPC.SetInt(slotWoken, 0)
			},
			OnTalk: stoneTalk,
		},
	}
}

// stoneTalk wakes the NPC global memo key "2" names. The key must hold a
// value: with none the hook aborts, leaving the stone in the world with
// every later talk ignored.
func stoneTalk(s *script.Script, e script.Talk) string {
	n := e.NPC
	if n.Int(slotWoken) == 0 {
		n.SetInt(slotWoken, 1)

		i0 := s.GlobalMemo().Int(memoKey, noMemo)
		if i0 == noMemo {
			panic(fmt.Sprintf("baium: global memo %q holds no value", memoKey))
		}
		if i0 != -1 {
			if c0 := s.Object(int32(i0)); c0 != nil {
				s.SendScriptEvent(c0.(*script.NPC), 10025, e.Player.ObjectID(), 0)
			}
		}
		n.DeleteMe()
	}
	return ""
}
