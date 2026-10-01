package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// CPRecoveryOutcome is how an arena manager's CP restore went.
type CPRecoveryOutcome int

const (
	// CPRecoveryUnpaid took nothing and says nothing: the fee could not be
	// taken although the talker held it.
	CPRecoveryUnpaid CPRecoveryOutcome = iota
	// CPRecoveryNotEnoughAdena took nothing: the talker holds less than the
	// fee.
	CPRecoveryNotEnoughAdena
	// CPRecoveryPaid took the fee, and the NPC will cast the restore on the
	// talker.
	CPRecoveryPaid
)

// CPRecoveryFee is the adena an arena manager's CP restore costs.
const CPRecoveryFee = 100

// arenaCPRecovery is the skill an arena manager restores CP with.
var arenaCPRecovery = modelskill.Ref{ID: 4380, Level: 1}

// cpRecoveryWeight is the weight of the restore's cast desire.
const cpRecoveryWeight = 1_000_000

// arenaManager reports whether npcID is one of the arena managers that
// restore CP.
func arenaManager(npcID int) bool { return npcID == 31225 || npcID == 31226 }

// CPRecovery takes the fee from talker, then has f cast its CP restore on
// talker on its next AI tick.
func (f *Folk) CPRecovery(talker *player.Character) CPRecoveryOutcome {
	inv := talker.Inventory()
	if inv == nil || inv.Adena() < CPRecoveryFee {
		return CPRecoveryNotEnoughAdena
	}
	if inv.DestroyByTemplateID(item.AdenaID, CPRecoveryFee) == nil {
		return CPRecoveryUnpaid
	}
	f.AddCastDesire(talker, arenaCPRecovery, cpRecoveryWeight)
	return CPRecoveryPaid
}
