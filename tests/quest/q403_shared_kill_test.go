package quest

import (
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q403"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Path to a Rogue (Q403_PathToARogue.onAttacked, onMyDying): a skeleton hit
// with Neti's weapon by two players of the quest drops no bone to either.
// A scenario cannot show it: its hit line has the character alone strike
// and a second player is not expressible (#4052).
const (
	q403Path       = "quest.Q403_PathToARogue"
	q403Name       = "Q403_PathToARogue"
	q403Skeleton   = 20035
	q403NetiDagger = 1182
	q403Bones      = 1183
)

// q403SharedKill boots two level 19 characters with the quest at cond 2,
// each wielding Neti's dagger, a skeleton that always drops its bone, and
// has the characters strike it in the order of strikers (true: the second
// character, false: the first); the last striker lands a lethal blow. It
// returns the server and the first character.
func q403SharedKill(t *testing.T, strikers []bool) (*gameservertest.Server, int32) {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	templates = append(templates,
		&item.Template{
			ID: q403NetiDagger, Name: "Neti's Dagger", Kind: item.KindWeapon, Slot: item.SlotRHand, Duration: -1,
			Weapon: &item.WeaponDetail{Type: item.WeaponDagger},
		},
		&item.Template{
			ID: q403Bones, Name: "Spartoi Bones", Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	table := item.NewTable(templates)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Hunter", 19, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(table),
		gameservertest.WithScriptRand(func(int) int { return 0 }),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q403Skeleton: script.KindHostile},
			[]script.Listing{{Path: q403Path}},
			script.Catalog{q403Path: q403.New},
		),
	)
	first := srv.SoleObjectID(t)
	second := srv.SeedCharacterFor(t, "mate", "Mate", 19, 0).ID
	for _, id := range []int32{first, second} {
		for _, r := range [][2]string{{"<state>", "STARTED"}, {"<cond>", "2"}} {
			if _, err := srv.DB.Exec("INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", id, q403Name, r[0], r[1]); err != nil {
				t.Fatal(err)
			}
		}
		tmpl, _ := table.Get(q403NetiDagger)
		inst := item.NewStackOrEquip(srv.NewObjectID(), tmpl, 1, true)
		if !inst.Equipped() {
			t.Fatal("Neti's dagger is not worn")
		}
		inst.OwnerID = id
		if err := srv.Items.Create(t.Context(), id, inst); err != nil {
			t.Fatal(err)
		}
	}
	enterWorld(t, srv)
	if len(strikers) > 0 && containsTrue(strikers) {
		mate := srv.DialClient(t, "mate", 1)
		mate.Send(encodeRequestGameStart(0))
		readUntil(t, mate, serverpackets.OpcodeCharSelected)
		mate.Send(encodeSingleOpcode(clientpackets.OpcodeEnterWorld))
		srv.ReadQueued(t, mate)
		srv.ReadQueued(t, srv.Client)
	}

	x, y, z := srv.PlayerPosition(t, first)
	skel := srv.SpawnHostileNPCTemplateAt(t, proofMonster(q403Skeleton), location.Location{X: x + 40, Y: y, Z: z})
	for i, byMate := range strikers {
		who := first
		if byMate {
			who = second
		}
		p := playerCombatant(t, srv, who)
		last := i == len(strikers)-1
		done := make(chan struct{})
		if !srv.PlayerQueue(t, who).Post(func() {
			defer close(done)
			dmg := 1
			if last {
				dmg = skel.MaxHP() + 1
			}
			skel.TakeDamage(dmg, p)
		}) {
			t.Fatal("the player's queue is closed")
		}
		<-done
	}
	srv.Advance(t, 3500*time.Millisecond)
	return srv, first
}

func containsTrue(bs []bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}

// TestQ403SharedKillDropsNoBone: the character who alone fights a skeleton
// with Neti's weapon gets the bone; once another player of the quest has
// struck it with Neti's weapon as well, the killing blow gives neither
// player a bone and the quest stays at cond 2.
func TestQ403SharedKillDropsNoBone(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		strikers []bool
		want     int
	}{
		{"alone", []bool{false, false}, 1},
		{"another player also strikes", []bool{false, true, false}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, first := q403SharedKill(t, tc.strikers)
			if got := srv.PlayerItemCount(t, first, q403Bones); got != tc.want {
				t.Fatalf("Spartoi bones after the kill = %d, want %d", got, tc.want)
			}
			var cond string
			if err := srv.DB.QueryRow("SELECT value FROM character_quests WHERE charId=? AND name=? AND var='<cond>'", first, q403Name).Scan(&cond); err != nil {
				t.Fatal(err)
			}
			if cond != strconv.Itoa(2) {
				t.Fatalf("cond = %s, want 2", cond)
			}
		})
	}
}
