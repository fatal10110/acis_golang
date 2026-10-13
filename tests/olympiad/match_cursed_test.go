package olympiad

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/grounditem"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// zaricheID is Demonic Sword Zariche, a cursed weapon.
const zaricheID int32 = 8190

// cursedMatchOptions run Zariche, worn in both hands as the datapack
// declares it, next to the match's Noblesse Gate Pass.
func cursedMatchOptions(t *testing.T) []gameservertest.Option {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for i, tmpl := range templates {
		if tmpl.ID == zaricheID {
			lr := *tmpl
			lr.Slot = item.SlotLRHand
			lr.Dropable, lr.Destroyable, lr.Tradable = false, false, false
			templates[i] = &lr
		}
	}
	templates = append(templates, &item.Template{
		ID: olympiad.NoblesseGatePass, Name: "Noblesse Gate Pass", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
		Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
	})
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID: zaricheID, Skill: modelskill.Ref{ID: 3603, Level: 3}, Name: "Demonic Sword Zariche",
		DropRate: 1, Duration: 72, DurationLost: 24, DisappearChance: 50, StageKills: 10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return []gameservertest.Option{
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCursedWeapons(table),
	}
}

// pickUpZariche lays Zariche at p's feet and has p pick it up, becoming
// its holder.
func (m *match) pickUpZariche(t *testing.T, p *competitor) {
	t.Helper()
	x, y, z := m.srv.PlayerPosition(t, p.id)
	m.srv.SeedGroundItem(t, 0, zaricheID, 1, x, y, z)
	var ground *grounditem.Item
	for _, obj := range m.srv.State.Objects() {
		if g, ok := obj.(*grounditem.Item); ok && g.ItemID() == zaricheID {
			ground = g
		}
	}
	if ground == nil {
		t.Fatal("no Zariche on the ground")
	}
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	for _, v := range []int{int(ground.ObjectID()), x, y, z} {
		w.WriteInt32(int32(v))
	}
	w.WriteUint8(0)
	p.c.Send(w.Bytes())
	m.srv.Settle(t)
	m.read(t)
	if c, ok := m.character(t, p).(interface{ CursedWeaponID() int32 }); !ok || c.CursedWeaponID() != zaricheID {
		t.Fatalf("%s does not hold Zariche after picking it up", p.name)
	}
	for _, q := range m.ones {
		q.frames = nil
	}
}

// heldItemMessage reports whether p was told id naming the item itemID.
func (p *competitor) heldItemMessage(id, itemID int32) bool {
	for _, f := range p.frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() != id || r.ReadInt32() != 1 || r.ReadInt32() != serverpackets.SystemMessageParamItemName {
			continue
		}
		if r.ReadInt32() == itemID {
			return true
		}
	}
	return false
}

// TestOlympiadMatchCursedHolderDefects pins a competitor that picked up a
// cursed weapon after registering: as the stadium opens it is told it
// cannot compete holding the weapon, named, and its opponent that the game
// was cancelled for an unfit opponent. The holder defects, losing a third
// of its points, and the match counts for no one; neither is brought in.
func TestOlympiadMatchCursedHolderDefects(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, nil, cursedMatchOptions(t)...)
	a, b := m.ones[0], m.ones[1]
	m.pickUpZariche(t, b)

	m.drawn(t)
	m.over(t)

	if want := lines(enterCountdown, "SM 1494", "SM 1658 Bravo 6", townCountdown, "Mode 0"); !slices.Equal(a.course(), want) {
		t.Errorf("Alpha course:\n got %q\nwant %q", a.course(), want)
	}
	// The defector is never released: it was never brought in.
	if want := lines(enterCountdown, "SM 1658 Bravo 6", townCountdown); !slices.Equal(b.course(), want) {
		t.Errorf("Bravo course:\n got %q\nwant %q", b.course(), want)
	}
	if !b.heldItemMessage(serverpackets.SystemMessageCannotJoinOlympiadPossessingS1, zaricheID) {
		t.Error("Bravo was not told it cannot compete holding Zariche")
	}
	if a.heldItemMessage(serverpackets.SystemMessageCannotJoinOlympiadPossessingS1, zaricheID) {
		t.Error("Alpha was told of Bravo's weapon")
	}
	if n, _ := m.srv.Olympiad.Noble(a.id); n.Points != 18 || n.CompDone != 0 {
		t.Errorf("Alpha record = %+v, want 18 points and no match", n)
	}
	if n, _ := m.srv.Olympiad.Noble(b.id); n.Points != 12 || n.CompDone != 0 {
		t.Errorf("Bravo record = %+v, want 12 points and no match", n)
	}
	if got := matchFights(t, m); len(got) != 0 {
		t.Fatalf("olympiad_fights = %+v, want none", got)
	}
}
