package network

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestGameClientLinkEnchantItemInGame(t *testing.T) {
	c, chars, items, state := newLinkedGameClient(t)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	objID := chars.soleObjectID(t)
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   504,
		TemplateID: 30,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed weapon: %v", err)
	}
	if err := items.Create(context.Background(), objID, item.Instance{
		ObjectID:   505,
		TemplateID: 955,
		OwnerID:    objID,
		Count:      1,
		Location:   item.LocationInventory,
	}); err != nil {
		t.Fatalf("seed scroll: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeUseItem(505, false))
	assertStaticSystemMessageFrame(t, c.Read(), serverpackets.SystemMessageSelectItemToEnchant)
	assertChooseInventoryItemFrame(t, c.Read(), 955)

	c.Send(encodeRequestEnchantItem(504))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("enchant success message opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	assertEnchantResultFrame(t, c.Read(), serverpackets.EnchantResultSuccess)
	if reply := c.Read(); reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("enchant userinfo opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
	}

	inventoryUpdatesFor(t, state).Tick()
	if reply := c.Read(); reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("enchant inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
}

func TestScheduleAfterRecoversPanickingCallback(t *testing.T) {
	buf := &syncBuffer{}
	l := &GameClientLink{log: zerolog.New(buf)}

	l.scheduleAfter(time.Millisecond, func() { panic("boom") })

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(buf.String(), "boom") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not recovered and logged, got: %s", buf.String())
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNewGameClientLinkCubicAfterFuncRecoversPanickingCallback proves the
// production cubicAfterFunc wired by NewGameClientLink (issue #830's
// acceptance criteria: "no remaining unrecovered time.AfterFunc callbacks
// in production code paths") recovers a panicking cubic fire/disappear
// callback and still runs a subsequently scheduled one, matching the
// attack/move recover-and-log pattern this closes the gap for.
func TestNewGameClientLinkCubicAfterFuncRecoversPanickingCallback(t *testing.T) {
	buf := &syncBuffer{}
	link := NewGameClientLink(GameClientLinkConfig{Log: zerolog.New(buf)})

	link.cubicAfterFunc(time.Millisecond, func() { panic("boom") })

	deadline := time.Now().Add(time.Second)
	for !strings.Contains(buf.String(), "cubic: recovered panic") {
		if time.Now().After(deadline) {
			t.Fatalf("panic was not recovered and logged, got: %s", buf.String())
		}
		time.Sleep(time.Millisecond)
	}

	fired := make(chan struct{})
	link.cubicAfterFunc(time.Millisecond, func() { close(fired) })
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatal("cubicAfterFunc did not fire a subsequent callback after a recovered panic")
	}
}

// syncBuffer is a mutex-guarded bytes.Buffer safe for a test's polling
// goroutine to read while a background goroutine writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
