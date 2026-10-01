package serverpackets

import (
	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
)

// Recipe book opcodes.
const (
	OpcodeRecipeBookItemList = 0xd6
	OpcodeRecipeItemMakeInfo = 0xd7
)

// Craft outcomes RecipeItemMakeInfo reports for the last attempt.
const (
	RecipeMakeInfoNone    int32 = -1
	RecipeMakeInfoFailed  int32 = 0
	RecipeMakeInfoSuccess int32 = 1
)

// FrameRecipeBookItemList builds the recipe book window for the dwarven or
// common page: its type (0 dwarven, 1 common), the crafter's max MP, then
// each recipe id with its 1-based position.
func FrameRecipeBookItemList(dwarven bool, maxMP int32, recipes []recipe.Recipe) wire.Frame {
	w := newFrameWriter(OpcodeRecipeBookItemList)
	if dwarven {
		w.WriteInt32(0)
	} else {
		w.WriteInt32(1)
	}
	w.WriteInt32(maxMP)
	w.WriteInt32(int32(len(recipes)))
	for i, r := range recipes {
		w.WriteInt32(int32(r.ID))
		w.WriteInt32(int32(i + 1))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameRecipeItemMakeInfo builds the craft window for r: its id, its page
// (0 dwarven, 1 common), the crafter's current and max MP, and status, the
// outcome of the attempt that sent it (RecipeMakeInfoNone when none was
// made).
func FrameRecipeItemMakeInfo(r recipe.Recipe, mp, maxMP, status int32) wire.Frame {
	w := newFrameWriter(OpcodeRecipeItemMakeInfo)
	w.WriteInt32(int32(r.ID))
	if r.Dwarven {
		w.WriteInt32(0)
	} else {
		w.WriteInt32(1)
	}
	w.WriteInt32(mp)
	w.WriteInt32(maxMP)
	w.WriteInt32(status)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
