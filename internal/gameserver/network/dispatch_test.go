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
	c, chars, items, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
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

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeUseItem(505, false))
	assertStaticSystemMessageFrame(t, c.read(), serverpackets.SystemMessageSelectItemToEnchant)
	assertChooseInventoryItemFrame(t, c.read(), 955)

	c.send(encodeRequestEnchantItem(504))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeSystemMessage {
		t.Fatalf("enchant success message opcode = %#x, want SystemMessage (%#x)", reply[0], serverpackets.OpcodeSystemMessage)
	}
	reply = c.read()
	if reply[0] != serverpackets.OpcodeInventoryUpdate {
		t.Fatalf("enchant inventory opcode = %#x, want InventoryUpdate (%#x)", reply[0], serverpackets.OpcodeInventoryUpdate)
	}
	assertEnchantResultFrame(t, c.read(), serverpackets.EnchantResultSuccess)
	if reply := c.read(); reply[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("enchant userinfo opcode = %#x, want UserInfo (%#x)", reply[0], serverpackets.OpcodeUserInfo)
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
