package npcs

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The private warehouse handles of the script-facing API freeze (#3807),
// as the Ketra and Varka support features call them: getWarehouse().getSize()
// and, for a stocked warehouse, ActionFailed, setActiveWarehouse and
// WarehouseWithdrawList(player, 1).

// TestScriptOpensPrivateWarehouseWithdraw: an empty private warehouse reads
// 0 and sends nothing. Once stocked it reads its row count, and in a new
// session, where no warehouse is active yet, the script's open releases
// the client and shows the same withdrawal list the keeper's WithdrawP
// shows, with the private warehouse active, so a withdrawal from it goes
// through.
func TestScriptOpensPrivateWarehouseWithdraw(t *testing.T) {
	t.Parallel()
	director := gameservertest.WithScripts([]script.Listing{{Path: "quest.Director"}}, script.Catalog{
		"quest.Director": func() script.Script { return script.Script{} },
	})
	w := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}}, director)
	size := func(w *whWorld) int {
		t.Helper()
		n := -1
		if !w.srv.RunScript(t, w.player, "Director", func(_ *script.Script, p *script.Player) { n = p.WarehouseSize() }) {
			t.Fatal("WarehouseSize panicked")
		}
		return n
	}

	if got := size(w); got != 0 {
		t.Fatalf("empty warehouse size = %d, want 0", got)
	}
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("reading the size sent %x", opcodes(frames))
	}

	w.command(t, "DepositP")
	sword, potion := w.items[swordID], w.items[potionID]
	w.c.Send(encodeDeposit(whRow{sword, 1}, whRow{potion, 4}))
	drainUntilQuiet(t, w.c)
	if got := size(w); got != 2 {
		t.Fatalf("stocked warehouse size = %d, want 2", got)
	}

	// A new session of the same character: nothing is active yet.
	c := w.srv.DialClient(t, w.srv.Account(), 1)
	w.srv.AdvanceUntil(t, "first session out of the world", func() bool {
		_, ok := w.srv.State.Player(w.player)
		return !ok
	})
	w.c = c
	w.enter(t)
	drainFrames(t, w.c)

	if !w.srv.RunScript(t, w.player, "Director", func(_ *script.Script, p *script.Player) {
		if p.WarehouseSize() != 0 {
			p.OpenWarehouseWithdraw()
		}
	}) {
		t.Fatal("OpenWarehouseWithdraw panicked")
	}
	frames := drainFrames(t, w.c)
	requireOpcodes(t, "script open", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList)
	scriptList := frames[1]
	whType, adena, _, rows := decodeWarehouseList(t, scriptList, serverpackets.OpcodeWarehouseWithdrawList)
	if whType != 1 || adena != whAdena-2*depositFee || len(rows) != 2 || rows[sword] != (listRow{swordID, 1}) {
		t.Fatalf("script withdraw list = type %d adena %d %v, want 1, %d, the sword and 4 potions", whType, adena, rows, whAdena-2*depositFee)
	}

	w.c.Send(encodeWithdraw(whRow{sword, 1}))
	drainUntilQuiet(t, w.c)
	if got := w.held(t, sword); got != 1 {
		t.Fatalf("sword held after the withdrawal = %d, want 1", got)
	}
	if got := rowsAt(saved(t, w.srv, w.player), item.LocationWarehouse); got[swordID] != 0 || got[potionID] != 4 {
		t.Fatalf("saved warehouse rows = %v, want the 4 potions alone", got)
	}

	keeper := w.command(t, "WithdrawP")
	requireOpcodes(t, "WithdrawP", keeper, serverpackets.OpcodeWarehouseWithdrawList, serverpackets.OpcodeActionFailed, serverpackets.OpcodeActionFailed)
	if !w.srv.RunScript(t, w.player, "Director", func(_ *script.Script, p *script.Player) { p.OpenWarehouseWithdraw() }) {
		t.Fatal("OpenWarehouseWithdraw panicked")
	}
	frames = drainFrames(t, w.c)
	requireOpcodes(t, "second script open", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseWithdrawList)
	if !bytes.Equal(frames[1], keeper[0]) {
		t.Fatalf("script withdraw list % x, want the keeper's % x", frames[1], keeper[0])
	}
}
