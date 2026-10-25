package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q212"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q213"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q215"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q216"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q217"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q218"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q222"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q223"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q224"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q225"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q241"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q242"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q246"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q247"
)

// quests201To250 is the catalog of quests 201 to 250 (#182), except the four the engine slices ported.
func quests201To250() script.Catalog {
	return script.Catalog{
		"quest.Q212_TrialOfDuty":              q212.New,
		"quest.Q213_TrialOfTheSeeker":         q213.New,
		"quest.Q214_TrialOfTheScholar":        nil,
		"quest.Q215_TrialOfThePilgrim":        q215.New,
		"quest.Q216_TrialOfTheGuildsman":      q216.New,
		"quest.Q217_TestimonyOfTrust":         q217.New,
		"quest.Q218_TestimonyOfLife":          q218.New,
		"quest.Q219_TestimonyOfFate":          nil,
		"quest.Q220_TestimonyOfGlory":         nil,
		"quest.Q221_TestimonyOfProsperity":    nil,
		"quest.Q222_TestOfTheDuelist":         q222.New,
		"quest.Q223_TestOfTheChampion":        q223.New,
		"quest.Q224_TestOfSagittarius":        q224.New,
		"quest.Q225_TestOfTheSearcher":        q225.New,
		"quest.Q226_TestOfTheHealer":          nil,
		"quest.Q227_TestOfTheReformer":        nil,
		"quest.Q228_TestOfMagus":              nil,
		"quest.Q229_TestOfWitchcraft":         nil,
		"quest.Q231_TestOfTheMaestro":         nil,
		"quest.Q232_TestOfTheLord":            nil,
		"quest.Q233_TestOfTheWarSpirit":       nil,
		"quest.Q241_PossessorOfAPreciousSoul": q241.New,
		"quest.Q242_PossessorOfAPreciousSoul": q242.New,
		"quest.Q246_PossessorOfAPreciousSoul": q246.New,
		"quest.Q247_PossessorOfAPreciousSoul": q247.New,
	}
}
