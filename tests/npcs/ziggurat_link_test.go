package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// destroyHeld destroys count units of the player's templateID through the
// client's destroy packet.
func (w *folkWorld) destroyHeld(t *testing.T, templateID, count int32) {
	t.Helper()
	var objectID int32
	w.srv.RunScript(t, w.player, "OracleTeleporter", func(_ *script.Script, p *script.Player) {
		for _, held := range p.HeldItems() {
			if held.ItemID == templateID {
				objectID = held.ObjectID
			}
		}
	})
	if objectID == 0 {
		t.Fatalf("the player holds no %d", templateID)
	}
	pw := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	pw.WriteInt32(objectID)
	pw.WriteInt32(count)
	w.c.Send(pw.Bytes())
	w.srv.ReadQueued(t, w.c)
}

// TestOracleTeleporterZigguratLinkRechecksAfterThePage: a level 70 player
// holding the fee and a dimension fragment is offered the rift. One that
// destroys the fragment, or adena below the fee, before following the link
// is answered the gatekeeper's refusal, pays nothing, stays where it is and
// is not remembered (owner-approved divergence, #5158); one that keeps both
// pays the fee once and goes, keeping the fragment.
func TestOracleTeleporterZigguratLinkRechecksAfterThePage(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		destroy map[int32]int32
		page    string
		left    map[int32]int
	}{
		{
			name: "fragment destroyed", destroy: map[int32]int32{tpFragment: 1}, page: "ziggurat_nofrag.htm",
			left: map[int32]int{int32(item.AdenaID): 30000, tpFragment: 0},
		},
		{
			name: "adena spent", destroy: map[int32]int32{int32(item.AdenaID): 29000}, page: "ziggurat_noadena.htm",
			left: map[int32]int{int32(item.AdenaID): 1000, tpFragment: 1},
		},
		{
			name: "both kept", page: "ziggurat_rift.htm",
			left: map[int32]int{int32(item.AdenaID): 5500, tpFragment: 1},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootOracles(t, oracleSetup{level: 70, held: map[int32]int32{int32(item.AdenaID): 30000, tpFragment: 1}})
			gk := w.spawnFolk(t, folkTemplate("DungeonGatekeeper", tpZiggurat), 50)
			w.openThrough(t, gk)
			wantTalkAnswer(t, w.tripFrames(t, npcCommand(gk, "Quest OracleTeleporter")), gk, teleportPage(t, gk, "OracleTeleporter", "ziggurat.htm"))
			for id, n := range tt.destroy {
				w.destroyHeld(t, id, n)
			}
			from := w.at

			frames := w.tripFrames(t, "Quest OracleTeleporter zigurratDimensional")
			page, _ := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
			if page == nil {
				t.Fatalf("the rift link answered no page: %x", opcodes(frames))
			}
			if _, html, _ := htmlMessage(t, page); html != teleportPage(t, gk, "OracleTeleporter", tt.page) {
				t.Fatalf("page = %q, want %s", html, tt.page)
			}
			if tt.destroy == nil {
				w.requireTrip(t, frames, tpRiftEntrance)
				w.requireSaved(t, tt.left)
				if got := w.oracleMemo(t); got == "" {
					t.Fatal("the gatekeeper is not remembered after the trip")
				}
				return
			}
			if got := systemMessageIDs(frames); len(got) != 0 {
				t.Fatalf("system messages = %v, want none", got)
			}
			w.requireNoTrip(t, frames)
			x, y, z := w.srv.PlayerPosition(t, w.player)
			if got := (location.Location{X: x, Y: y, Z: z}); got != from {
				t.Fatalf("player at %+v after the refusal, want %+v", got, from)
			}
			w.requireSaved(t, tt.left)
			if got := w.oracleMemo(t); got != "" {
				t.Fatalf("saved memo = %q, want none", got)
			}
		})
	}
}
