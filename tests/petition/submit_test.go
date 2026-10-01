package petition

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestPetitionNeedsAListedGM pins RequestPetition.java:31-36: with no game
// master listed the petition is refused with NO_GM_PROVIDING_SERVICE_NOW
// and its sound.
func TestPetitionNeedsAListedGM(t *testing.T) {
	t.Parallel()
	r := boot(t)
	enterWorld(t, r.player)
	frames := exchange(t, r.player, encodePetition("help", 3))
	testsupport.AssertOpcodeSequence(t, frames, serverpackets.OpcodeSystemMessage, serverpackets.OpcodePlaySound)
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessageNoGMProvidingServiceNow))
	if len(r.srv.Petitions.List()) != 0 {
		t.Fatal("a refused petition was filed")
	}
}

// TestPetitionSubmit pins RequestPetition.java:38-77 and
// PetitionManager.submitPetition: the refusals in order (an over-long text,
// a type the client cannot name), the accepted petition's three answers,
// the game masters' notice, and the one-active-petition refusal.
func TestPetitionSubmit(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)

	assertMessages(t, exchange(t, r.player, encodePetition(strings.Repeat("a", 256), 3)), msg(serverpackets.SystemMessagePetitionMaxChars255))
	// The reference throws on a type out of range and answers nothing; the
	// petition window still waits on an answer.
	testsupport.AssertOpcodeSequence(t, exchange(t, r.player, encodePetition("help", 10)), serverpackets.OpcodeActionFailed)

	frames := exchange(t, r.player, encodePetition(strings.Repeat("a", 255), 3))
	list := r.srv.Petitions.List()
	if len(list) != 1 || list[0].Type != petition.TypeBugReport || list[0].State != petition.Pending || list[0].Petitioner != r.playerID {
		t.Fatalf("petitions = %+v, want one pending bug report by Player", list)
	}
	id := list[0].ID
	assertMessages(t, frames,
		msg(serverpackets.SystemMessagePetitionAcceptedRecentNoS1, id),
		msg(serverpackets.SystemMessageSubmittedYourS1ThPetitionS2Left, int32(1), int32(4)),
		msg(serverpackets.SystemMessageS1PetitionOnWaitingList, int32(1)))
	assertSays(t, collect(t, r.gm), say{r.playerID, sayHeroVoice, "Petition System", "Player has submitted a new petition."})

	assertMessages(t, exchange(t, r.player, encodePetition("again", 3)), msg(serverpackets.SystemMessageOnlyOneActivePetitionAtTime))
	assertSays(t, collect(t, r.gm))
}

// TestPetitionDisabled pins PetitioningAllowed = False: every petition is
// refused with GAME_CLIENT_UNABLE_TO_CONNECT_TO_PETITION_SERVER.
func TestPetitionDisabled(t *testing.T) {
	t.Parallel()
	r := boot(t, gameservertest.WithPetitionConfig(petition.Config{MaxPerPlayer: 5, MaxPending: 25}))
	r.enterAll(t)
	assertMessages(t, exchange(t, r.player, encodePetition("help", 3)), msg(serverpackets.SystemMessageGameClientUnableToConnectToPetitionServer))
}

// TestPetitionLimits pins the two counts RequestPetition.java:50-62 checks:
// a cancelled petition does not count toward MaxPetitionsPerPlayer but a
// closed one does, and MaxPetitionsPending counts every active petition on
// the server.
func TestPetitionLimits(t *testing.T) {
	t.Parallel()
	r := boot(t, gameservertest.WithPetitionConfig(petition.Config{Allowed: true, MaxPerPlayer: 1, MaxPending: 1}))
	r.enterAll(t)

	r.submit(t)
	assertMessages(t, exchange(t, r.player, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionCanceledSubmitS1MoreToday, int32(1)))
	collect(t, r.gm)

	frames := exchange(t, r.player, encodePetition("help", 3))
	if len(frames) != 3 {
		t.Fatalf("resubmit frames = %x, want the three acceptance messages", testsupport.FrameOpcodes(frames))
	}
	assertMessages(t, frames[1:2], msg(serverpackets.SystemMessageSubmittedYourS1ThPetitionS2Left, int32(1), int32(0)))
	collect(t, r.gm)

	// The game master's own petition finds the server full.
	assertMessages(t, exchange(t, r.gm, encodePetition("mine", 3)), msg(serverpackets.SystemMessagePetitionSystemCurrentUnavailable))

	id := r.srv.Petitions.List()[1].ID
	exchange(t, r.gm, encodeBypass("admin_petition join "+itoa(id)))
	exchange(t, r.gm, encodePetitionCancel())
	drain(t, r.player)
	assertMessages(t, exchange(t, r.player, encodePetition("help", 3)), msg(serverpackets.SystemMessageWeHaveReceivedS1PetitionsToday, int32(2)))
}

// TestPetitionCancel pins RequestPetitionCancel.java:45-54: a pending
// petition is cancelled with the count still allowed and the game masters'
// notice; with none, PETITION_NOT_SUBMITTED.
func TestPetitionCancel(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	id := r.submit(t)

	assertMessages(t, exchange(t, r.player, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionCanceledSubmitS1MoreToday, int32(5)))
	assertSays(t, collect(t, r.gm), say{r.playerID, sayHeroVoice, "Petition System", "Player has canceled a pending petition."})
	if list := r.srv.Petitions.List(); len(list) != 1 || list[0].ID != id || list[0].State != petition.Cancelled {
		t.Fatalf("petitions = %+v, want petition %d cancelled", list, id)
	}
	assertMessages(t, exchange(t, r.player, encodePetitionCancel()), msg(serverpackets.SystemMessagePetitionNotSubmitted))
}
