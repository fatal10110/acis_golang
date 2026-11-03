package npcs

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// TestAdminRenamedNPCInfoCrest pins the NpcInfo //set name shows a castle
// NPC again with (AdminEditChar.java admin_set name, new NpcInfo(npc,
// null)): with ShowNpcCrest it keeps the owner's crest.
func TestAdminRenamedNPCInfoCrest(t *testing.T) {
	t.Parallel()
	admin, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	seed := crestClanSeed(t, taxCastleID)
	w := bootCrestWorld(t, true,
		gameservertest.WithShowNPCCrest(),
		gameservertest.WithAdmin(admin),
		gameservertest.WithClanSeed(func(db *sql.DB) {
			seed(db)
			if _, err := db.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE char_name = 'Talker'"); err != nil {
				t.Fatalf("grant GM: %v", err)
			}
		}),
	)
	f, _ := w.spawnFolkFrames(t, folkTemplate("Merchant", merchantID), 50)
	w.c.Send(encodeAction(f.ObjectID(), w.at, false))
	drainFrames(t, w.c)

	cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	cmd.WriteString("set name Steward")
	w.c.Send(cmd.Bytes())
	if got := npcCrestFields(t, npcInfoOf(t, drainFrames(t, w.c), f.ObjectID())); got != ownerCrest {
		t.Fatalf("renamed NpcInfo crest fields = %#x, want %#x", got, ownerCrest)
	}
	if got := f.CharacterName(); got != "Steward" {
		t.Fatalf("NPC name = %q, want the new one", got)
	}
}
