package npcs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: RequestBypassToServer npc_ (Integer.parseInt of the object
// id), Npc.onBypassFeedback Chat (Integer.parseInt(command.substring(5)))
// and WarehouseKeeper.onBypassFeedback FreightChar (Integer.parseInt of the
// text after the last '_'). Integer.parseInt reads any Basic Multilingual
// Plane decimal digit: a Java probe (OpenJDK 21) prints 12 for both
// Integer.parseInt("１２") and Integer.parseInt("١٢").

const (
	fullwidthZero   = '０'
	arabicIndicZero = '٠'
)

// digitsIn writes n with the decimal digits that start at zero.
func digitsIn(n int32, zero rune) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return zero + r - '0'
		}
		return r
	}, strconv.Itoa(int(n)))
}

// TestBypassNpcReadsUnicodeDigits pins a dialog link written in fullwidth
// or Arabic-Indic digits, as an edit box can feed one: the object id and
// the chat page number read as their ASCII forms, so the link opens the
// page it names; a digit outside the Basic Multilingual Plane is no digit,
// so such an object id drops the command silently.
func TestBypassNpcReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, dialogPages(), noBypassReuse)
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)
	drainUntilQuiet(t, w.c)

	for _, zero := range []rune{fullwidthZero, arabicIndicZero} {
		w.openAnyNpcPage(t)
		command := "npc_" + digitsIn(f.ObjectID(), zero) + "_Chat " + digitsIn(2, zero)
		assertAnswer(t, w.bypass(t, command), chatWindowAnswer, f, wantChatPage(dialogPage2, f))
	}

	w.openAnyNpcPage(t)
	assertAnswer(t, w.bypass(t, "npc_"+digitsIn(f.ObjectID(), '\U0001D7CE')+"_Chat 2"), nil, f, "")
}

// TestFreightCharReadsUnicodeDigits pins FreightChar_<id> with the
// receiver's id in fullwidth and in Arabic-Indic digits: each opens the
// receiver's freight deposit list.
func TestFreightCharReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	w := bootFreight(t, whAdena, [][2]int32{{swordID, 1}})

	for _, zero := range []rune{fullwidthZero, arabicIndicZero} {
		frames := w.command(t, "FreightChar_"+digitsIn(w.receiver.ID, zero))
		requireOpcodes(t, "FreightChar_ in non-ASCII digits", frames, serverpackets.OpcodeActionFailed, serverpackets.OpcodeWarehouseDepositList, serverpackets.OpcodeActionFailed)
	}
}
