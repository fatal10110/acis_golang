package petition

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/social/petition"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// petitionRow is the list row AdminPetition.showPendingPetitions writes for
// an unread pending bug report by an online Player that the game master
// does not answer, on an even line.
func petitionRow(id int32) string {
	return `<table width=280 height=40 bgcolor=000000><tr><td width=20 align=center><img src="L2UI_CH3.msnicon1" width=12 height=16>` +
		`<img src="L2UI_CH3.QuestWndInfoIcon_5" width=11 height=16></td><td width=260><a action="bypass -h admin_petition view ` + itoa(id) +
		`">#` + itoa(id) + ` by Player</a><br1><font color=B09878>Type:</font> BUG_REPORT <font color=B09878>State:</font> PENDING</td>` +
		`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
}

// onePageBar is Pagination.generatePages for page 1 of 1.
const onePageBar = `<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="bypass admin_petition 1" back=L2UI_CH3.prev1_down fore=L2UI_CH3.prev1 width=16 height=16></td>` +
	`<td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td>` +
	`<td FIXWIDTH=26 align=center><font color=LEVEL>01</font></td>` +
	`<td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td><td FIXWIDTH=26 align=center></td>` +
	`<td FIXWIDTH=22 align=center><img height=2><button action="bypass admin_petition 1" back=L2UI_CH3.next1_down fore=L2UI_CH3.next1 width=16 height=16></td></tr></table>` +
	`<img src="L2UI.SquareGray" width=280 height=1>`

// TestAdminPetitionList pins //petition's list page: one row per petition,
// padded to seven rows, then the page bar; a page below 1 opens nothing.
func TestAdminPetitionList(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	id := r.submit(t)

	frames := exchange(t, r.gm, encodeBuildCmd("petition"))
	if len(frames) != 1 {
		t.Fatalf("list frames = %x, want the list page", testsupport.FrameOpcodes(frames))
	}
	content := petitionRow(id) + strings.Repeat("<img height=41>", 6) + onePageBar
	page := assertPage(t, frames[0], "\t"+content+"\n")
	if strings.Contains(page, "Unfollow") {
		t.Fatalf("list offers unfollow to a game master answering nothing: %q", page)
	}
	if frames := exchange(t, r.gm, encodeBuildCmd("petition 0")); len(frames) != 0 {
		t.Fatalf("page 0 frames = %x, want none", testsupport.FrameOpcodes(frames))
	}
	// Viewing marks the petition read.
	exchange(t, r.gm, encodeBuildCmd("petition view "+itoa(id)))
	assertPage(t, exchange(t, r.gm, encodeBuildCmd("petition 5"))[0], `<img src="L2UI_CH3.party_styleicon1_2" width=11 height=16>`)

	for _, command := range []string{"petition join", "petition join x", "petition bogus"} {
		frames := exchange(t, r.gm, encodeBuildCmd(command))
		if len(frames) != 2 {
			t.Fatalf("%s frames = %x, want usage then the list", command, testsupport.FrameOpcodes(frames))
		}
		assertMessages(t, frames[:1], msg(serverpackets.SystemMessageS1, "Usage: //petition [join|reject|reset|show|unfollow|view]"))
	}
}

// TestAdminPetitionActions pins //petition unfollow, show, a join of a
// petition with chat, reject and reset (AdminPetition.java,
// Petition.abortConsultation and endConsultation).
func TestAdminPetitionActions(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	id := r.submit(t)
	join := encodeBypass("admin_petition join " + itoa(id))
	exchange(t, r.gm, join)
	collect(t, r.player)
	line := say{r.playerID, sayPetitionPlayer, "Player", "hello"}
	exchange(t, r.player, encodeSay2(sayPetitionPlayer, line.text))
	collect(t, r.gm)

	// The last game master leaving puts the petition back to pending.
	frames := exchange(t, r.gm, encodeBypass("admin_petition unfollow"))
	if len(frames) != 2 {
		t.Fatalf("unfollow frames = %x, want the end notice then the list", testsupport.FrameOpcodes(frames))
	}
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessagePetitionEndedWithS1, "Player"))
	assertMessages(t, collect(t, r.player), msg(serverpackets.SystemMessageS1LeftPetitionChat, "The GM"))
	if list := r.srv.Petitions.List(); list[0].State != petition.Pending {
		t.Fatalf("state after unfollow = %v, want PENDING", list[0].State)
	}

	frames = exchange(t, r.gm, encodeBypass("admin_petition show "+itoa(id)))
	assertSays(t, frames[:1], line)
	htmlBody(t, frames[1])

	// Joining a petition with chat shows the joiner the chat and tells
	// nobody.
	frames = exchange(t, r.gm, join)
	assertSays(t, frames[:1], line)
	htmlBody(t, frames[1])
	if frames := collect(t, r.player); len(frames) != 0 {
		t.Fatalf("petitioner frames on a rejoin = %x, want none", testsupport.FrameOpcodes(frames))
	}

	// A reset waits for every petition being answered.
	assertMessages(t, exchange(t, r.gm, encodeBypass("admin_petition reset")), msg(serverpackets.SystemMessagePetitionUnderProcess))

	frames = exchange(t, r.gm, encodeBypass("admin_petition reject "+itoa(id)))
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessagePetitionEndedWithS1, "Player"))
	assertPage(t, frames[1], "State:</font> REJECTED")
	if frames := collect(t, r.player); len(frames) != 0 {
		t.Fatalf("petitioner frames on a reject = %x, want none", testsupport.FrameOpcodes(frames))
	}
	frames = exchange(t, r.gm, encodeBypass("admin_petition reject "+itoa(id)))
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessageFailedCancelPetitionTryLater))
	frames = exchange(t, r.gm, encodeBypass("admin_petition join "+itoa(id)))
	assertMessages(t, frames[:1], msg(serverpackets.SystemMessageNotUnderPetitionConsultation))

	frames = exchange(t, r.gm, encodeBypass("admin_petition reset"))
	if len(frames) != 1 || strings.Contains(htmlBody(t, frames[0]), "admin_petition view") {
		t.Fatalf("reset frames = %x, want an empty list", testsupport.FrameOpcodes(frames))
	}
	if list := r.srv.Petitions.List(); len(list) != 0 {
		t.Fatalf("petitions after reset = %+v, want none", list)
	}
}

// TestAdminForcePetitionAndAddChat pins //force_peti and //add_peti_chat
// with each refusal and error number (AdminPetition.java:100-163), a
// player added to the chat, its lines, and its leaving
// (RequestPetitionCancel.java:39-40).
func TestAdminForcePetitionAndAddChat(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	helper, helperID := r.addPlayer(t, "player3", "Helper")

	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("force_peti")), msg(serverpackets.SystemMessageClientNotLoggedOntoGameServer))
	exchange(t, r.gm, encodeAction(r.gmID))
	drain(t, r.player)
	drain(t, helper)
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("force_peti")), msg(serverpackets.SystemMessagePetitionFailedForS1ErrorNumberS2, "Admin", int32(1)))
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("add_peti_chat")), msg(serverpackets.SystemMessagePetitionAddingS1FailedErrorNumberS2, "Admin", int32(1)))

	exchange(t, r.gm, encodeAction(r.playerID))
	drain(t, r.player)
	drain(t, helper)
	frames := exchange(t, r.gm, encodeBuildCmd("force_peti"))
	list := r.srv.Petitions.List()
	if len(list) != 1 || list[0].Type != petition.TypeOther || list[0].State != petition.Accepted {
		t.Fatalf("petitions = %+v, want one accepted OTHER petition", list)
	}
	id := list[0].ID
	if len(frames) != 2 {
		t.Fatalf("force frames = %x, want the notice then the code", testsupport.FrameOpcodes(frames))
	}
	assertSays(t, frames[:1], say{r.playerID, sayHeroVoice, "Petition System", "Player has submitted a new petition."})
	assertMessages(t, frames[1:], msg(serverpackets.SystemMessagePetitionS1ReceivedCodeIsS2, "Admin", id))
	assertMessages(t, collect(t, r.player), msg(serverpackets.SystemMessageS1ReceivedConsultationRequest, "Player"))

	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("force_peti")), msg(serverpackets.SystemMessagePetitionFailedS1AlreadySubmitted, "Player"))
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("add_peti_chat")), msg(serverpackets.SystemMessagePetitionAddingS1FailedErrorNumberS2, "Player", int32(3)))

	exchange(t, r.gm, encodeAction(helperID))
	drain(t, r.player)
	drain(t, helper)
	participating := msg(serverpackets.SystemMessageS1ParticipatePetition, "Helper")
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("add_peti_chat")), participating)
	assertMessages(t, collect(t, r.player), participating)
	assertMessages(t, collect(t, helper), participating)
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("add_peti_chat")), msg(serverpackets.SystemMessagePetitionAddingS1FailedErrorNumberS2, "Helper", int32(4)))

	// A responder that is no game master speaks on the player channel.
	line := say{helperID, sayPetitionPlayer, "Helper", "same thing happened to me"}
	assertSays(t, exchange(t, helper, encodeSay2(sayPetitionGM, line.text)), line)
	assertSays(t, collect(t, r.gm), line)
	assertSays(t, collect(t, r.player), line)

	// A responder that is no game master leaves the chat on cancel.
	left := msg(serverpackets.SystemMessageS1LeftPetitionChat, "Helper")
	assertMessages(t, exchange(t, helper, encodePetitionCancel()), left)
	assertMessages(t, collect(t, r.gm), left)
	assertMessages(t, collect(t, r.player), left)

	exchange(t, r.gm, encodeBypass("admin_petition unfollow"))
	drain(t, r.player)
	assertMessages(t, exchange(t, r.gm, encodeBuildCmd("add_peti_chat")), msg(serverpackets.SystemMessagePetitionAddingS1FailedErrorNumberS2, "Helper", int32(2)))
}

// TestPetitionTextCannotCarryLinks pins the guard on player-written text a
// game master's petition window shows: a link in the petition text loses
// its action, so it cannot run a command as the game master who clicks it.
func TestPetitionTextCannotCarryLinks(t *testing.T) {
	t.Parallel()
	r := boot(t)
	r.enterAll(t)
	exchange(t, r.player, encodePetition(`<a action="bypass -h admin_set access 8">Join</a>`, 3))
	collect(t, r.gm)
	id := r.srv.Petitions.List()[0].ID
	page := htmlBody(t, exchange(t, r.gm, encodeBypass("admin_petition view "+itoa(id)))[0])
	if strings.Contains(page, "bypass -h admin_set") {
		t.Fatalf("petition page carries the player's link: %q", page)
	}
	if !strings.Contains(page, `<a =" -h admin_set access 8">Join</a>`) {
		t.Fatalf("petition page = %q, want the text without its link words", page)
	}
}
