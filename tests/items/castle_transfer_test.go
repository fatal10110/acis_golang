package items

import (
	"context"
	"database/sql"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The rival clan: "Rival" leads rivalClanID, a level 5 clan owning no
// castle.
const (
	rivalClanID = 0x70000015
	rivalID     = 0x70000014

	wyvernMountNPCID  int32 = 12621
	striderMountNPCID int32 = 12526 // Wind Strider

	siegeVictoryFile = "Siege_Victory"
)

// seedRivalClan clones Newbie's character row as "Rival" on account
// "rival", leading the clan "Rivals".
func seedRivalClan(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx) // the temporary table lives on one connection
	if err != nil {
		t.Fatalf("seed rival clan: %v", err)
	}
	defer conn.Close()
	for _, s := range []struct {
		q    string
		args []any
	}{
		{"CREATE TEMPORARY TABLE rival_seed SELECT * FROM characters WHERE char_name = 'Newbie'", nil},
		{"UPDATE rival_seed SET account_name = 'rival', obj_Id = ?, char_name = 'Rival', clanid = ?", []any{rivalID, rivalClanID}},
		{"INSERT INTO characters SELECT * FROM rival_seed", nil},
		{"DROP TEMPORARY TABLE rival_seed", nil},
		{"INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id) VALUES (?, 'Rivals', 5, 0, ?)", []any{rivalClanID, rivalID}},
	} {
		if _, err := conn.ExecContext(ctx, s.q, s.args...); err != nil {
			t.Fatalf("seed rival clan: %s: %v", s.q, err)
		}
	}
}

// mountedLeader is the slice of a live player the transfer scenario
// drives: its mount.
type mountedLeader interface {
	Mount(npcID, controlItemID int32) bool
	MountType() int32
}

func liveMount(t *testing.T, w *castleWorld, objID int32) mountedLeader {
	t.Helper()
	obj, ok := w.srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	m, ok := obj.(mountedLeader)
	if !ok {
		t.Fatalf("player %d = %T has no mount state", objID, obj)
	}
	return m
}

// clanFrames returns, among frames, the clan ids of the
// PledgeShowInfoUpdate frames with their castle ids, and the PlaySound
// files.
func clanFrames(frames [][]byte) (pledges map[int32]int32, sounds []string) {
	pledges = map[int32]int32{}
	for _, f := range frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodePledgeShowInfoUpdate:
			clanID := r.ReadInt32()
			r.ReadInt32()
			r.ReadInt32()
			pledges[clanID] = r.ReadInt32()
		case serverpackets.OpcodePlaySound:
			r.ReadInt32()
			sounds = append(sounds, r.ReadString())
		}
	}
	return pledges, sounds
}

// dismountOf reports whether frames hold the dismount Ride of objID.
func dismountOf(frames [][]byte, objID int32) bool {
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeRide {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() == objID && r.ReadInt32() == 0 {
			return true
		}
	}
	return false
}

// TestAdminCastleSetTransfersOwnedCastle drives //castle set on Gludio
// Castle, owned by the Lords, for the Rivals' leader. The castle changes
// hands: the Lords' clan and clan_data row lose it, the Rivals' gain it,
// and only the Rivals' members see the clan header refresh and hear the
// siege victory music. The Lords' leader, online, is dismounted when
// riding a wyvern (mount type 2) and stays on a strider.
func TestAdminCastleSetTransfersOwnedCastle(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name       string
		mountNPC   int32
		mountType  int32
		dismounted bool
	}{
		{"wyvern rider dismounted", wyvernMountNPCID, player.MountTypeWyvern, true},
		{"strider rider stays mounted", striderMountNPCID, player.MountTypeStrider, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootCastleWorld(t, 1, func(db *sql.DB) { seedRivalClan(t, db) })
			w.enter(t)
			rival := w.srv.DialClient(t, "rival", 1)
			enterAsClanMember(rival)
			drainUntilQuiet(t, rival)
			drainUntilQuiet(t, w.leader)
			drainUntilQuiet(t, w.gm)

			leader := liveMount(t, w, w.leaderID)
			if !leader.Mount(tt.mountNPC, 1) || leader.MountType() != tt.mountType {
				t.Fatalf("leader mount type = %d, want %d", leader.MountType(), tt.mountType)
			}
			drainUntilQuiet(t, w.leader)

			w.gm.Send(encodeAction(rivalID, 0, 0, 0, false))
			for _, c := range []*testsupport.ScriptedClient{w.gm, w.leader, rival} {
				drainUntilQuiet(t, c)
			}

			cmd := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
			cmd.WriteString("castle set gludio_castle")
			w.gm.Send(cmd.Bytes())
			gm, lords, rivals := collectFrames(w.gm), collectFrames(w.leader), collectFrames(rival)

			if pledges, sounds := clanFrames(rivals); len(pledges) != 1 || pledges[rivalClanID] != 1 || len(sounds) != 1 || sounds[0] != siegeVictoryFile {
				t.Fatalf("rival leader: pledges %v, sounds %q, opcodes %x; want Rivals castle 1 and Siege_Victory", pledges, sounds, opcodesOf(rivals))
			}
			if pledges, sounds := clanFrames(lords); len(pledges) != 0 || len(sounds) != 0 {
				t.Fatalf("former leader: pledges %v, sounds %q; want neither", pledges, sounds)
			}
			if got := dismountOf(lords, w.leaderID); got != tt.dismounted {
				t.Fatalf("former leader dismount Ride = %v, want %v (opcodes %x)", got, tt.dismounted, opcodesOf(lords))
			}
			if got := dismountOf(gm, w.leaderID); got != tt.dismounted {
				t.Fatalf("game master saw the dismount = %v, want %v", got, tt.dismounted)
			}
			wantType := tt.mountType
			if tt.dismounted {
				wantType = 0
			}
			if got := leader.MountType(); got != wantType {
				t.Fatalf("former leader mount type = %d, want %d", got, wantType)
			}

			for clanID, want := range map[int32]int32{castleClanID: 0, rivalClanID: 1} {
				cl, ok := w.srv.Clans.Table().Get(clanID)
				if !ok {
					t.Fatalf("clan %#x missing", clanID)
				}
				if got := cl.CastleID(); got != want {
					t.Fatalf("clan %#x castle = %d, want %d", clanID, got, want)
				}
			}
			gludio, ok := w.srv.Castles.Get(1)
			if !ok {
				t.Fatal("Gludio Castle missing")
			}
			if got := gludio.OwnerID(); got != rivalClanID {
				t.Fatalf("Gludio owner = %#x, want %#x", got, rivalClanID)
			}
			for clanID, want := range map[int32]int64{castleClanID: 0, rivalClanID: 1} {
				if got := queryCastleInt(t, w.srv, "SELECT hasCastle FROM clan_data WHERE clan_id = ?", clanID); got != want {
					t.Fatalf("clan %#x hasCastle = %d, want %d", clanID, got, want)
				}
			}
		})
	}
}
