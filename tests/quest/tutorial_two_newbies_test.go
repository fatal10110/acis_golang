package quest

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// TestTutorialFirstStepKeepsOtherNewbiesSteps pins the owner-approved
// divergence of #5137: a newbie's first tutorial step cancels only its own
// pending tutorial timer. Two new characters enter five seconds apart; the
// first one's step at 10 s used to cancel every player's pending step, so
// the second newbie never saw its first voice and page. Each now gets its
// first step 10 s after its own entry and its second voice 30 s later.
func TestTutorialFirstStepKeepsOtherNewbiesSteps(t *testing.T) {
	t.Parallel()
	srv := bootNewbie(t, true, false)
	first := createNewbie(t, srv)
	enterBurst(t, srv)
	srv.Advance(t, 5*time.Second)
	if got := tutorialFrames(srv.ReadQueued(t, srv.Client)); len(got) != 0 {
		t.Fatalf("first newbie before its step: sent %x, want nothing", got)
	}

	c := srv.DialClient(t, "newbie2", 0)
	c.Send(encodeRequestCharacterCreate("Newbie2"))
	readUntil(t, c, serverpackets.OpcodeCharCreateOk)
	readUntil(t, c, serverpackets.OpcodeCharSelectInfo)
	var second int32
	if err := srv.DB.QueryRow("SELECT obj_Id FROM characters WHERE char_name='Newbie2'").Scan(&second); err != nil {
		t.Fatal(err)
	}
	c.Send(clientwire.RequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(clientwire.SingleOpcode(clientpackets.OpcodeEnterWorld))
	srv.ReadQueued(t, c)

	var firstAt, secondAt location.Location
	onCharacter(t, srv, first, func(p *player.Character) { firstAt = p.CurrentLocation() })
	onCharacter(t, srv, second, func(p *player.Character) { secondAt = p.CurrentLocation() })

	srv.Advance(t, 5*time.Second)
	expectFrames(t, "10 s: the first newbie's first step", tutorialFrames(srv.ReadQueued(t, srv.Client)),
		voiceFrame(first, firstAt, "tutorial_voice_001a"), tutorialPageFrame(t, "tutorial_human_fighter001.htm"))
	expectFrames(t, "10 s: the second newbie", tutorialFrames(srv.ReadQueued(t, c)))

	srv.Advance(t, 5*time.Second)
	expectFrames(t, "15 s: the first newbie", tutorialFrames(srv.ReadQueued(t, srv.Client)))
	expectFrames(t, "15 s: the second newbie's first step", tutorialFrames(srv.ReadQueued(t, c)),
		voiceFrame(second, secondAt, "tutorial_voice_001a"), tutorialPageFrame(t, "tutorial_human_fighter001.htm"))

	srv.Advance(t, 25*time.Second)
	expectFrames(t, "40 s: the first newbie's second voice", tutorialFrames(srv.ReadQueued(t, srv.Client)),
		voiceFrame(first, firstAt, "tutorial_voice_002"))
	expectFrames(t, "40 s: the second newbie", tutorialFrames(srv.ReadQueued(t, c)))

	srv.Advance(t, 5*time.Second)
	expectFrames(t, "45 s: the first newbie", tutorialFrames(srv.ReadQueued(t, srv.Client)))
	expectFrames(t, "45 s: the second newbie's second voice", tutorialFrames(srv.ReadQueued(t, c)),
		voiceFrame(second, secondAt, "tutorial_voice_002"))
}
