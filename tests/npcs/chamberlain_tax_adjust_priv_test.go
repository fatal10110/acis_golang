package npcs

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// TestChamberlainTaxAdjustPrivilege pins the tax privilege on tax_adjust:
// a member without it gets the tax_set refusal (noprivs.htm, then
// tax-member.htm) whatever rate it names, and the stored next rate stays;
// a member holding it sets the rate. The check is an owner-approved
// divergence (#5014): the reference lets any member reaching the
// chamberlain change the next tax rate.
func TestChamberlainTaxAdjustPrivilege(t *testing.T) {
	t.Parallel()
	t.Run("member without the tax privilege", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: privCastleFunctions, strife: sevensigns.Dawn}})
		for _, command := range []string{"tax_adjust 20", "tax_adjust", "tax_adjust x", "tax_adjust 26"} {
			frames := w.run(t, command)
			if got, want := opcodes(frames), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}; !bytes.Equal(got, want) {
				t.Fatalf("%s = % x, want % x", command, got, want)
			}
			if _, got, _ := htmlMessage(t, frames[0]); got != w.want("noprivs.htm") {
				t.Fatalf("%s refusal page = %q", command, got)
			}
			if _, got, _ := htmlMessage(t, frames[1]); got != w.want("tax-member.htm", "%currentTaxRate%", "15", "%nextTaxRate%", "15", "%objectId%", w.oid()) {
				t.Fatalf("%s tax-member page = %q", command, got)
			}
		}
		if _, next, _, _, _ := w.castleRow(t); next != 15 {
			t.Fatalf("stored next tax %d, want 15", next)
		}
	})
	t.Run("member with the tax privilege", func(t *testing.T) {
		t.Parallel()
		w := bootChamberlain(t, commandSetup{residenceSetup: residenceSetup{rank: 5, privs: privCastleTaxes, strife: sevensigns.Dawn}})
		assertAnswer(t, w.run(t, "tax_adjust 20"), pageThenFinal, w.f, w.want("tax-after.htm", "%nextTaxRate%", "20", "%objectId%", w.oid()))
		if _, next, _, _, _ := w.castleRow(t); next != 20 {
			t.Fatalf("stored next tax %d, want 20", next)
		}
	})
}
