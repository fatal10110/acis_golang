// Package bossmemo is the table of global memo keys under which the grand
// bosses and their lair NPCs leave their object ids. Each key has exactly
// one owner: the NPC that stores its object id there while it lives. Every
// script that reads or writes one of these keys names it from this table.
package bossmemo

// The global memo keys, by owner.
const (
	// AntharasSmall, AntharasNormal and AntharasLarge hold the three sizes
	// of Antharas. Antharas stores itself under its GM_ID spawn parameter,
	// which the lair's spawns set to these values.
	AntharasSmall  = "1"
	AntharasNormal = "9"
	AntharasLarge  = "10"

	// Baium holds Baium.
	Baium = "2"
	// Valakas holds Valakas.
	Valakas = "3"

	// Frintezza holds Frintezza, FrintezzaFollower the follower he summons
	// and FrintezzaTransform its transformed form.
	Frintezza          = "4"
	FrintezzaFollower  = "5"
	FrintezzaTransform = "6"

	// SculptureGarden holds the sculpture in Ice Queen Freya's garden.
	SculptureGarden = "7"
)
