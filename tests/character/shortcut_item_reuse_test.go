package character

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: ShortCutInit.java:52-92 and ShortCutRegister.java:40-85 write,
// for an etc item whose item_skill list is set, the player's reuse
// timestamp keyed by the first item skill's reuse hash code:
// (int)(remaining/1000), then (int)(skill.getReuseDelay()/1000); 0,0 when
// no timestamp is recorded. Player._reuseTimeStamps loses entries only on
// Player.deleteMe's clear (Player.java:5850; enableSkill at :2198 has no
// caller), so a timestamp that already ran out still writes 0 and the
// skill's delay for the rest of the session.

// shortcutPotionSkillReuse is the Lesser Healing Potion skill's reuse in the
// table below, shorter than the potion's own 10 s reuse_delay: the timer
// runs for the item's delay, while the shortcut's second field carries the
// skill's.
const shortcutPotionSkillReuse = 3000

func shortcutPotionSkills(t *testing.T) *skillstate.Persistence {
	t.Helper()
	db := sqltest.SharedDB(t)
	return skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: 2031, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, StaticReuse: true, ReuseDelay: shortcutPotionSkillReuse,
		},
	}), gamesql.NewCharacterSkillStore(db))
}

func encodeShortcutUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

// itemShortcutReuse is the reuse part of one item shortcut entry.
type itemShortcutReuse struct {
	remaining, reuse, augmentation int32
}

// itemShortcutReuseIn returns the reuse fields of the item shortcut pointing
// at objectID in a ShortCutInit or ShortCutRegister frame.
func itemShortcutReuseIn(t *testing.T, frame []byte, objectID int32) itemShortcutReuse {
	t.Helper()
	r := wire.NewReader(frame[1:])
	var n int32
	switch frame[0] {
	case serverpackets.OpcodeShortCutInit:
		n = r.ReadInt32()
	case serverpackets.OpcodeShortCutRegister:
		n = 1
	default:
		t.Fatalf("opcode = %#x, want ShortCutInit or ShortCutRegister", frame[0])
	}
	for range n {
		typ := serverpackets.ShortcutType(r.ReadInt32())
		r.ReadInt32() // slot
		id := r.ReadInt32()
		switch typ {
		case serverpackets.ShortcutItem:
			r.ReadInt32() // character type
			r.ReadInt32() // shared reuse group
			got := itemShortcutReuse{remaining: r.ReadInt32(), reuse: r.ReadInt32(), augmentation: r.ReadInt32()}
			if id == objectID {
				if err := r.Err(); err != nil {
					t.Fatalf("parse shortcut: %v", err)
				}
				return got
			}
		case serverpackets.ShortcutSkill:
			r.ReadInt32() // level
			r.ReadUint8()
			r.ReadInt32() // character type
		default:
			r.ReadInt32() // character type
		}
	}
	t.Fatalf("no item shortcut for object %d (parse error: %v)", objectID, r.Err())
	return itemShortcutReuse{}
}

// registerItemShortcut registers objectID on page 0, slot slot, and returns
// the ShortCutRegister answer.
func registerItemShortcut(t *testing.T, c *testsupport.ScriptedClient, slot, objectID int32) []byte {
	t.Helper()
	c.Send(encodeRequestShortCutReg(int32(serverpackets.ShortcutItem), wireShortcutSlot(0, slot), objectID, 1))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeShortCutRegister {
		t.Fatalf("register opcode = %#x, want ShortCutRegister (%#x)", reply[0], serverpackets.OpcodeShortCutRegister)
	}
	return reply
}

// TestItemShortcutCarriesItemSkillReuse drives an item shortcut's reuse
// fields over the wire: zeros before the potion's first use and for an etc
// item without item skills; the time left and the item skill's own delay
// after a use, on ShortCutRegister and again on the relog's ShortCutInit
// from the restored timer; and 0 with the skill's delay once that timer ran
// out, even after an autosave read the timers.
func TestItemShortcutCarriesItemSkillReuse(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithSkills(shortcutPotionSkills(t)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithCapturedLog(),
		gameservertest.WithReuseDelays(0, 0),
	)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("server log:\n%s", srv.LogText())
		}
	})
	c, objID := srv.Client, srv.SoleObjectID(t)
	potion := srv.GiveItem(t, objID, lesserHealingPotionID, 5)
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	for _, sc := range []shortcut.Shortcut{
		{Slot: 1, Page: 0, Type: shortcut.Item, ID: potion, Level: -1, CharacterType: 1},
		{Slot: 2, Page: 0, Type: shortcut.Item, ID: adena, Level: -1, CharacterType: 1},
	} {
		if err := srv.Shortcuts.Save(context.Background(), objID, 0, sc); err != nil {
			t.Fatalf("seed shortcut %+v: %v", sc, err)
		}
	}
	const skillReuseSeconds = shortcutPotionSkillReuse / 1000

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	if got := itemShortcutReuseIn(t, frames[11], potion); got != (itemShortcutReuse{}) {
		t.Fatalf("ShortCutInit potion before any use = %+v, want zeros", got)
	}
	if got := itemShortcutReuseIn(t, frames[11], adena); got != (itemShortcutReuse{}) {
		t.Fatalf("ShortCutInit adena = %+v, want zeros", got)
	}
	drainQuiet(t, c)

	// The use arms a 10 s timer (the potion's reuse_delay); the shortcut's
	// second field is the item skill's 3 s.
	c.Send(encodeShortcutUseItem(potion))
	srv.Advance(t, time.Second)
	drainQuiet(t, c)
	if got := itemShortcutReuseIn(t, registerItemShortcut(t, c, 4, potion), potion); got.remaining < 8 || got.remaining > 9 ||
		got.reuse != skillReuseSeconds || got.augmentation != 0 {
		t.Fatalf("ShortCutRegister potion one second after use = %+v, want 8..9 s left, reuse %d, no augmentation", got, skillReuseSeconds)
	}
	if got := itemShortcutReuseIn(t, registerItemShortcut(t, c, 5, adena), adena); got != (itemShortcutReuse{}) {
		t.Fatalf("ShortCutRegister adena = %+v, want zeros", got)
	}

	// The logout saves the timer; the relog restores it before ShortCutInit.
	c.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout opcode = %#x, want LeaveWorld (%#x)", reply[0], serverpackets.OpcodeLeaveWorld)
	}
	c.ExpectClosed()
	srv.FlushPersistence(t)

	relog := srv.DialClient(t, "player1", 1)
	relog.Send(encodeRequestGameStart(0))
	relog.Read() // SSQInfo
	relog.Read() // CharSelected
	relog.Send(encodeEnterWorld())
	frames = readEnterWorldBurst(t, relog)
	if got := itemShortcutReuseIn(t, frames[11], potion); got.remaining < 1 || got.remaining > 9 || got.reuse != skillReuseSeconds {
		t.Fatalf("relog ShortCutInit potion = %+v, want 1..9 s left and reuse %d", got, skillReuseSeconds)
	}
	drainQuiet(t, relog)

	// Once the timer ran out, the entry stays recorded for the session —
	// an autosave reading the pending timers does not drop it — and the
	// shortcut shows 0 left with the skill's delay.
	srv.Advance(t, 10*time.Second)
	srv.TickAutosave(t)
	drainQuiet(t, relog)
	if got := itemShortcutReuseIn(t, registerItemShortcut(t, relog, 6, potion), potion); got != (itemShortcutReuse{reuse: skillReuseSeconds}) {
		t.Fatalf("ShortCutRegister potion after the timer ran out = %+v, want 0 left and reuse %d", got, skillReuseSeconds)
	}
}
