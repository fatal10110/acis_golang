package party

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// zaricheID is the cursed weapon a festival monster can drop.
const zaricheID int32 = 8190

// zaricheTable runs Zariche as cursedWeapons.xml ships it.
func zaricheTable(t *testing.T) *entity.CursedWeaponTable {
	t.Helper()
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{
		ItemID:          zaricheID,
		Skill:           modelskill.Ref{ID: 3603, Level: 3},
		Name:            "Demonic Sword Zariche",
		DropRate:        1,
		Duration:        72,
		DurationLost:    24,
		DisappearChance: 50,
		StageKills:      10,
	}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable: %v", err)
	}
	return table
}

// A participant picking up a cursed weapon inside its arena is expelled
// from its party as it becomes the holder (CursedWeapon.activate ->
// Party.removePartyMember, EXPELLED); the party edit takes it out of the
// arena inside that call (updateParticipants), so its teleport and the
// removal text come before HAVE_BEEN_EXPELLED_FROM_PARTY and the cleared
// party window.
func TestFestivalParticipantPickingUpCursedWeaponIsRemovedFirst(t *testing.T) {
	t.Parallel()
	f := bootFestivalWith(t, festivalSetup{
		seats:      []seat{{"Leader", 20}, {"Ann", 20}, {"Bob", 20}, {"Cid", 20}},
		blueStones: 1000,
		minPlayers: 3,
		extra:      []gameservertest.Option{gameservertest.WithCursedWeapons(zaricheTable(t))},
	})
	for i := 1; i < 4; i++ {
		f.invite(t, 0, i, 0)
	}
	f.signUp(t)
	f.begin(t, 0, 1, 2, 3)

	bob := f.players[2]
	frames := f.pickUp(t, 2, 0, zaricheID, 1)[2]
	if got, want := removalSteps(frames, bob.id), []string{"teleport", "removed", "expelled", "windows"}; !slices.Equal(got, want) {
		t.Fatalf("Bob picking up Zariche = %v, want %v", got, want)
	}
	if got := f.participants(); !sameBools(got, []bool{true, true, false, true}) {
		t.Fatalf("participants after Bob's pickup = %v", got)
	}
	if got := f.character(t, 2).CursedWeaponID(); got != zaricheID {
		t.Fatalf("Bob holds cursed weapon %d, want Zariche", got)
	}
}
