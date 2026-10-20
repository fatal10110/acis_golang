// Package hall contains the siegable clan hall NPC behaviors: the
// defenders a hall siege raises, such as the Partisan Hideaway's Bloody
// Lord Nurka with his escort, guards, soldiers, sorcerers and healers,
// the Devastated Castle's Gustav with his lieutenants Dietrich and
// Mikhail, and the Fortress of the Dead's Lidia von Hellmann with Alfred
// and Giselle; and the NPCs of Rainbow Springs' hot springs game: its
// gourds, treasure chests and sweeper.
package hall

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// dir is the page directory of the hall behaviors.
const dir = "ai/siegeablehall"

// damageWeight is the attack desire weight of points against n: scale for
// each twentieth of n's maximum hit points they come to.
func damageWeight(n *script.NPC, points int32, scale float64) float64 {
	return float64(points) / float64(n.MaxHP()) / 0.05 * scale
}

// level1 is level 1 of skill id.
func level1(id skill.ID) skill.Ref { return skill.Ref{ID: id, Level: 1} }
