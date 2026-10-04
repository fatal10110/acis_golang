package items

import (
	"context"
	"database/sql"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// Shipped items gated on the user's clan rank or its clan's residences.
const (
	circletOfInnadrilID int32 = 6834 // <and> castle="6" pledgeClass="2"
	castlePledgeShield  int32 = 7015 // castle="-1"
	pledgeShieldID      int32 = 6902 // clanHall="-1"
	apellaHelmID        int32 = 7860 // pledgeClass="4"
	bsoeClanHallID      int32 = 5858 // skill 2177: clanHall="-1"
	bsoeCastleID        int32 = 5859 // skill 2178: castle="-1"

	residenceClanID = 0x70000001
)

// residenceClan is the clan the character leads: its level, the castle in
// clan_data.hasCastle and the hall whose clanhall row it owns (0 for none).
type residenceClan struct {
	level, castle, hall int
}

// bootResidenceItems boots "Newbie" leading clan (no clan when nil) against
// the shipped skill table and the residence-gated shipped items.
func bootResidenceItems(t *testing.T, clan *residenceClan) *gameservertest.Server {
	t.Helper()
	datapack.Require(t)
	skills, shippedItems := shippedData()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{circletOfInnadrilID, castlePledgeShield, pledgeShieldID, apellaHelmID, bsoeClanHallID, bsoeCastleID} {
		tmpl, ok := shippedItems.Get(id)
		if !ok {
			t.Fatalf("shipped item %d missing", id)
		}
		templates = append(templates, tmpl)
	}
	db := sqltest.SharedDB(t)
	opts := []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), skills, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 40, 0),
		gameservertest.WithWantChars(1),
	}
	if clan != nil {
		opts = append(opts, gameservertest.WithClanSeed(func(db *sql.DB) {
			ctx := context.Background()
			stmts := []struct {
				q    string
				args []any
			}{
				{"UPDATE characters SET clanid = ? WHERE char_name = 'Newbie'", []any{residenceClanID}},
				{`INSERT INTO clan_data (clan_id, clan_name, clan_level, hasCastle, leader_id)
					SELECT ?, 'Lords', ?, ?, obj_Id FROM characters WHERE char_name = 'Newbie'`, []any{residenceClanID, clan.level, clan.castle}},
			}
			if clan.hall != 0 {
				stmts = append(stmts, struct {
					q    string
					args []any
				}{"INSERT INTO clanhall (id, ownerId, paid) VALUES (?, ?, 1)", []any{clan.hall, residenceClanID}})
			}
			for _, s := range stmts {
				if _, err := db.ExecContext(ctx, s.q, s.args...); err != nil {
					t.Fatalf("seed residence clan: %v", err)
				}
			}
		}))
	}
	return gameservertest.Boot(t, opts...)
}

// enterAsClanMember enters the world without pinning the burst, which a
// clan member's pledge packets lengthen.
func enterAsClanMember(c *testsupport.ScriptedClient) {
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeEnterWorld())
}

// TestResidenceItemConditionsGateEquip pins the item <cond> clan
// predicates (ConditionPlayerHasCastle, ConditionPlayerHasClanHall,
// ConditionPlayerPledgeClass) on the equip path: castle="-1" and
// clanHall="-1" need the clan to own one, castle="6" that exact castle,
// pledgeClass="n" a clan rank of at least n, and nothing passes without a
// clan. A failed condition answers 1518 and leaves the item in the bag.
func TestResidenceItemConditionsGateEquip(t *testing.T) {
	t.Parallel()
	lord := &residenceClan{level: 5, castle: 6}      // leader rank 4
	otherLord := &residenceClan{level: 5, castle: 5} // leader rank 4
	hallOwner := &residenceClan{level: 0, hall: 22}  // leader rank 1
	noResidence := &residenceClan{level: 5}          // leader rank 4
	lowRank := &residenceClan{level: 4, castle: 6}   // leader rank 3
	for _, tt := range []struct {
		name  string
		clan  *residenceClan
		item  int32
		equip bool
	}{
		{"no clan, any castle", nil, castlePledgeShield, false},
		{"no clan, any hall", nil, pledgeShieldID, false},
		{"no clan, rank 4", nil, apellaHelmID, false},
		{"no residence, any castle", noResidence, castlePledgeShield, false},
		{"no residence, any hall", noResidence, pledgeShieldID, false},
		{"no residence, rank 4", noResidence, apellaHelmID, true},
		{"castle owner, any castle", lord, castlePledgeShield, true},
		{"castle owner, any hall", lord, pledgeShieldID, false},
		{"castle 6 owner, castle 6 circlet", lord, circletOfInnadrilID, true},
		{"castle 5 owner, castle 6 circlet", otherLord, circletOfInnadrilID, false},
		{"hall owner, any hall", hallOwner, pledgeShieldID, true},
		{"hall owner, any castle", hallOwner, castlePledgeShield, false},
		{"rank 1, rank 4", hallOwner, apellaHelmID, false},
		{"rank 3, rank 4", lowRank, apellaHelmID, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootResidenceItems(t, tt.clan)
			c, objID := srv.Client, srv.SoleObjectID(t)
			obj := srv.GiveItem(t, objID, tt.item, 1)
			enterAsClanMember(c)
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(obj, false))
			if tt.equip {
				// The equip answers with UserInfo and the slot updates.
				drainUntilQuiet(t, c)
			} else {
				assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotEquipItemDueToBadCondition)
				barrier(t, c)
			}
			srv.FlushItems(t)
			want := item.LocationInventory
			if tt.equip {
				want = item.LocationPaperdoll
			}
			if inst := mustFindItem(t, srv, objID, obj); inst.Location != want {
				t.Fatalf("item %d location after use = %v, want %v", tt.item, inst.Location, want)
			}
		})
	}
}

// TestResidenceScrollConditionsReadOwnership pins the skill <cond> side of
// the same predicates: Blessed Scroll of Escape: Clan Hall (2177,
// clanHall="-1") and: Castle (2178, castle="-1") start their cast for a
// clan that owns that kind of residence and answer S1_CANNOT_BE_USED naming
// the skill otherwise, consuming nothing.
func TestResidenceScrollConditionsReadOwnership(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		clan    *residenceClan
		item    int32
		skillID int32
		cast    bool
	}{
		{"hall scroll, hall owner", &residenceClan{hall: 22}, bsoeClanHallID, 2177, true},
		{"hall scroll, castle owner", &residenceClan{castle: 3}, bsoeClanHallID, 2177, false},
		{"hall scroll, no clan", nil, bsoeClanHallID, 2177, false},
		{"castle scroll, castle owner", &residenceClan{castle: 3}, bsoeCastleID, 2178, true},
		{"castle scroll, hall owner", &residenceClan{hall: 22}, bsoeCastleID, 2178, false},
		{"castle scroll, no clan", nil, bsoeCastleID, 2178, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := bootResidenceItems(t, tt.clan)
			c, objID := srv.Client, srv.SoleObjectID(t)
			scroll := srv.GiveItem(t, objID, tt.item, 2)
			enterAsClanMember(c)
			drainUntilQuiet(t, c)

			c.Send(encodeUseItem(scroll, false))
			frame := c.Read()
			if tt.cast {
				assertFrameOpcode(t, frame, serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
				if _, _, sid, _, _, _ := decodeMagicSkillUse(frame); sid != tt.skillID {
					t.Fatalf("MagicSkillUse skill = %d, want %d", sid, tt.skillID)
				}
				return
			}
			assertSystemMessageSkill(t, frame, serverpackets.SystemMessageS1CannotBeUsed, tt.skillID, 1)
			assertConditionRejectionOnly(t, srv, tt.name)
			srv.FlushItems(t)
			if inst := mustFindItem(t, srv, objID, scroll); inst.Count != 2 {
				t.Fatalf("scroll count after condition rejection = %d, want 2", inst.Count)
			}
		})
	}
}
