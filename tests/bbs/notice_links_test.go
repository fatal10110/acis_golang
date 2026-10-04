package bbs

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLoginClanNoticeNestedLinkWords pins the clan notice hardening
// (#3214): a notice hiding the link words inside themselves, or split by
// a backslash the window drops, reaches the login window without them, so
// it shows no live link and its page gives the member no bypass to send.
// The reference removes each word once, before the window's substitution,
// and lets both shapes through as a working link.
func TestLoginClanNoticeNestedLinkWords(t *testing.T) {
	_, c := bootAlone(t,
		gameservertest.WithCommunityBoard(boardOn),
		seedClan(t, true, `<a acactiontion="bypbypassass -h npc_1_x">a</a><a a\ction="b\ypass -h npc_2_y">b</a>`),
	)
	tail := burstTail(t, enterWorld(t, c))
	assertTail(t, tail, serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSkillCoolTime, serverpackets.OpcodeActionFailed)
	got := loginHTML(t, tail[0])
	want := `<html><title>Wolves</title><body><a =" -h npc_1_x">a</a><a =" -h npc_2_y">b</a></body></html>` + "\n"
	if got != want {
		t.Fatalf("clan notice = %q, want %q", got, want)
	}
	if strings.Contains(got, "bypass") || strings.Contains(got, "action") {
		t.Fatalf("clan notice %q still carries a link word", got)
	}
}
