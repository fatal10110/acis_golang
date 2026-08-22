package npc

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// TestNewHostileSeedsCalculatedMaxHP pins #1596: a template whose CON
// deviates from the calculator-neutral value must spawn at its calculated
// Max HP/MP (CreatureStatus.setMaxHpMp uses getMaxHp/getMaxMp, not the raw
// template value).
func TestNewHostileSeedsCalculatedMaxHP(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, MEN: 20, HPMax: 100, MPMax: 100})
	if got, want := h.MaxHP(), 80; got != want {
		t.Fatalf("MaxHP() = %d, want %d", got, want)
	}
	if got, want := h.CurrentHP(), h.MaxHP(); got != want {
		t.Fatalf("CurrentHP() = %d, want spawn at MaxHP() %d", got, want)
	}
	if got, want := h.MPValue(), h.MaxMPValue(); got != want {
		t.Fatalf("MPValue() = %v, want spawn at MaxMPValue() %v", got, want)
	}
}

// TestHostileSetCurrentHPClampsToCalculatedMaxHP pins #1596: restoring a
// persisted HP above the calculated Max HP must clamp to it, not the raw
// template value.
func TestHostileSetCurrentHPClampsToCalculatedMaxHP(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, HPMax: 100})
	h.SetCurrentHP(100)
	if got, want := h.CurrentHP(), h.MaxHP(); got != want {
		t.Fatalf("CurrentHP() after over-max SetCurrentHP = %d, want clamped to MaxHP() %d", got, want)
	}
}

// TestHostileBroadcastStatusSendsCalculatedMaxHP pins #1596: BroadcastStatus
// must send the calculated Max HP, not the raw template value.
func TestHostileBroadcastStatusSendsCalculatedMaxHP(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, HPMax: 100})
	state := world.New()
	h.SetWorld(state)
	state.Spawn(h, 0, 0, 0, 0)
	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)

	if err := h.BroadcastStatus(); err != nil {
		t.Fatalf("BroadcastStatus() error: %v", err)
	}
	if len(observer.frames) != 1 {
		t.Fatalf("frames after BroadcastStatus = %d, want 1", len(observer.frames))
	}
	frame := observer.frames[0]
	if got := binary.LittleEndian.Uint32(frame[13:17]); got != uint32(h.MaxHP()) {
		t.Fatalf("StatusUpdate max HP = %d, want calculated MaxHP() %d", got, h.MaxHP())
	}
}

// TestHostileTickRegenAddsHPAndMPUpToMax pins #1596's missing NPC HP/MP
// regeneration lifecycle (CreatureStatus.doRegeneration).
func TestHostileTickRegenAddsHPAndMPUpToMax(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, MEN: 20, HPMax: 100, MPMax: 100, HPRegen: 5, MPRegen: 3})
	h.SetHP(0)
	h.ReduceMP(h.MPValue())

	h.TickRegen()

	if got, want := h.HP(), h.HPRegenRate(); got != want {
		t.Fatalf("HP() after one regen tick = %v, want %v", got, want)
	}
	if got, want := h.MPValue(), h.MPRegenRate(); got != want {
		t.Fatalf("MPValue() after one regen tick = %v, want %v", got, want)
	}

	h.SetHP(h.MaxHPValue())
	h.AddMP(h.MaxMPValue())
	h.TickRegen()
	if got, want := h.HP(), h.MaxHPValue(); got != want {
		t.Fatalf("HP() at max after regen tick = %v, want unchanged %v", got, want)
	}
	if got, want := h.MPValue(), h.MaxMPValue(); got != want {
		t.Fatalf("MPValue() at max after regen tick = %v, want unchanged %v", got, want)
	}
}

// TestHostileTickRegenBroadcastsStatus pins #1596: a regen step that
// changes HP/MP announces the resulting HP to known observers.
func TestHostileTickRegenBroadcastsStatus(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, HPMax: 100, HPRegen: 5})
	state := world.New()
	h.SetWorld(state)
	state.Spawn(h, 0, 0, 0, 0)
	observer := &frameReceiver{trackedID: 2}
	state.Spawn(observer, 600, 0, 0, 0)
	h.SetHP(1)

	h.TickRegen()

	if len(observer.frames) != 1 {
		t.Fatalf("frames after TickRegen = %d, want 1", len(observer.frames))
	}
	if got := observer.frames[0][0]; got != serverpackets.OpcodeStatusUpdate {
		t.Fatalf("regen frame opcode = %#x, want StatusUpdate %#x", got, serverpackets.OpcodeStatusUpdate)
	}
}

// TestHostileTickRegenNoopOnceDead pins #1596: a dead NPC's HP/MP no longer
// regenerates (CreatureStatus.startHpMpRegeneration's !isDead() gate).
func TestHostileTickRegenNoopOnceDead(t *testing.T) {
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", CON: 20, HPMax: 100, HPRegen: 5})
	h.SetHP(1)
	h.MarkDead()

	h.TickRegen()

	if got, want := h.HP(), 1.0; got != want {
		t.Fatalf("HP() for dead NPC after TickRegen = %v, want unchanged %v", got, want)
	}
}
