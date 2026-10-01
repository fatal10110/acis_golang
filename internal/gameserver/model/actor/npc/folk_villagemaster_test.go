package npc

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

func villageMaster(t *testing.T, kind string) *Folk {
	t.Helper()
	inst, err := NewInstance(1, &Template{ID: 30704, TemplateID: 30704, Type: kind, Level: 1, HPMax: 100})
	if err != nil {
		t.Fatalf("%s: new instance: %v", kind, err)
	}
	f, err := NewFolk(inst, false)
	if err != nil {
		t.Fatalf("%s: new folk: %v", kind, err)
	}
	return f
}

// TestVillageMasterTeachesItsRaceAndLine pins which classes each master
// handles: the generic one every class, the dark elf, dwarf and orc ones
// their race's, and the fighter, mystic and priest ones the human and
// elven classes of their teaching line.
func TestVillageMasterTeachesItsRaceAndLine(t *testing.T) {
	const (
		gladiator       = 2  // human fighter
		sorcerer        = 12 // human mystic
		bishop          = 16 // human priest
		swordSinger     = 21 // elven fighter
		spellsinger     = 27 // elven mystic
		elvenElder      = 30 // elven priest
		shillienKnight  = 33 // dark elf fighter
		spellhowler     = 40 // dark elf mystic
		shillienElder   = 43 // dark elf priest
		destroyer       = 46 // orc fighter
		warcryer        = 52 // orc mystic
		bountyHunter    = 55 // dwarven fighter
		unknownClass    = 200
		duelist         = 88 // human fighter, third profession
		spectralMaster  = 111
		cardinal        = 97 // human priest, third profession
		dominator       = 115
		fortuneSeeker   = 117
		elementalMaster = 104 // elven mystic, third profession
	)
	all := []int{
		gladiator, sorcerer, bishop, swordSinger, spellsinger, elvenElder, shillienKnight, spellhowler, shillienElder,
		destroyer, warcryer, bountyHunter, duelist, spectralMaster, cardinal, dominator, fortuneSeeker, elementalMaster,
	}
	for _, tc := range []struct {
		kind    string
		teaches []int
		unknown bool
	}{
		{"VillageMaster", all, true},
		{"VillageMasterDElf", []int{shillienKnight, spellhowler, shillienElder, spectralMaster}, false},
		{"VillageMasterDwarf", []int{bountyHunter, fortuneSeeker}, false},
		{"VillageMasterOrc", []int{destroyer, warcryer, dominator}, false},
		{"VillageMasterFighter", []int{gladiator, swordSinger, duelist}, false},
		{"VillageMasterMystic", []int{sorcerer, spellsinger, elementalMaster}, false},
		{"VillageMasterPriest", []int{bishop, elvenElder, cardinal}, false},
	} {
		f := villageMaster(t, tc.kind)
		for _, id := range all {
			if got, want := f.Teaches(id), slices.Contains(tc.teaches, id); got != want {
				t.Errorf("%s.Teaches(%d) = %v, want %v", tc.kind, id, got, want)
			}
		}
		if got := f.Teaches(unknownClass); got != tc.unknown {
			t.Errorf("%s.Teaches(unknown) = %v, want %v", tc.kind, got, tc.unknown)
		}
	}
}

// TestVillageMasterOffersTheSubclassesItTeaches pins the add menu's filter:
// the base class's subclasses this master teaches, less any class a held
// subclass is or upgrades from.
func TestVillageMasterOffersTheSubclassesItTeaches(t *testing.T) {
	const gladiator = 2
	delf := villageMaster(t, "VillageMasterDElf")
	if got, want := delf.AvailableSubclasses(gladiator, nil), []int{33, 34, 36, 37, 40, 41, 43}; !slices.Equal(got, want) {
		t.Fatalf("dark elf master offers a Gladiator %v, want %v", got, want)
	}
	// A held Spellhowler takes it off the list; so does a held Spectral
	// Dancer (107), which upgrades from Bladedancer (34).
	held := []player.SubClass{{ClassID: 40, Index: 1}, {ClassID: 107, Index: 2}}
	if got, want := delf.AvailableSubclasses(gladiator, held), []int{33, 36, 37, 41, 43}; !slices.Equal(got, want) {
		t.Fatalf("dark elf master offers a Gladiator holding Spellhowler and Spectral Dancer %v, want %v", got, want)
	}
	if delf.ValidNewSubclass(gladiator, held, 40) || delf.ValidNewSubclass(gladiator, nil, 12) || !delf.ValidNewSubclass(gladiator, held, 41) {
		t.Fatal("dark elf master ValidNewSubclass: want a held class and a human class refused, Phantom Summoner allowed")
	}
	// An elven base is offered no dark elf class, even at the dark elf
	// master; a Paladin no Shillien Knight.
	if got := delf.AvailableSubclasses(21, nil); len(got) != 0 {
		t.Fatalf("dark elf master offers a Sword Singer %v, want nothing", got)
	}
	if got, want := delf.AvailableSubclasses(5, nil), []int{34, 36, 37, 40, 41, 43}; !slices.Equal(got, want) {
		t.Fatalf("dark elf master offers a Paladin %v, want %v", got, want)
	}
	priest := villageMaster(t, "VillageMasterPriest")
	if got, want := priest.AvailableSubclasses(gladiator, nil), []int{16, 17, 30}; !slices.Equal(got, want) {
		t.Fatalf("priest master offers a Gladiator %v, want %v", got, want)
	}
}
