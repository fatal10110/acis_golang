package quest

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// TestScriptClassChangeWithoutTemplate pins the defensive branch of the
// script class operations: a class whose template is not loaded changes
// nothing, neither the class nor the base class, and sends no frame. The
// reference loads every class's template, so it never reaches this branch.
func TestScriptClassChangeWithoutTemplate(t *testing.T) {
	// The Knight: the boot's template table holds no template for it.
	const knight = 4
	t.Parallel()
	srv, objID := bootHelpers(t)
	srv.ReadQueued(t, srv.Client)
	run(t, srv, objID, func(_ *script.Script, p *script.Player) {
		prev := p.ClassID()
		p.SetClassID(knight)
		p.SetBaseClass(knight)
		if p.ClassID() != prev || p.BaseClass() != prev {
			t.Errorf("class %d base %d, want both unchanged %d", p.ClassID(), p.BaseClass(), prev)
		}
	})
	interest := []byte{serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeUserInfo, serverpackets.OpcodeCharInfo}
	if got := framesOf(srv.ReadQueued(t, srv.Client), interest...); len(got) != 0 {
		t.Errorf("frames %x, want none", opcodesOf(got))
	}
}
