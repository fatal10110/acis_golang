package network

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestBypassWhitelistRecord(t *testing.T) {
	tests := []struct {
		name  string
		html  string
		allow []string
		deny  []string
	}{
		{
			name:  "exact link without -h",
			html:  `<a action="bypass npc_1_Chat 1">x</a>`,
			allow: []string{"npc_1_Chat 1"},
			deny:  []string{"npc_1_Chat 10", "npc_1_Chat", "npc_2_Chat 1"},
		},
		{
			name:  "exact link with -h",
			html:  `<a action="bypass -h npc_1_Chat 2">x</a>`,
			allow: []string{"npc_1_Chat 2"},
			deny:  []string{"-h npc_1_Chat 2", "h npc_1_Chat 2"},
		},
		{
			name:  "exact entry is trimmed",
			html:  `<a action="bypass  npc_1_Chat 1 ">x</a>`,
			allow: []string{"npc_1_Chat 1"},
			deny:  []string{" npc_1_Chat 1 ", "npc_1_Chat 12"},
		},
		{
			name:  "prefix entry is trimmed",
			html:  `<edit var="x"><a action="bypass -h  npc_1_Link $x ">x</a>`,
			allow: []string{"npc_1_Link a.htm", "npc_1_Link"},
			deny:  []string{"npc_2_Link a.htm", " npc_1_Link a.htm"},
		},
		{
			name: "empty prefix after -h is dropped",
			html: `<a action="bypass -h $x">x</a>`,
			deny: []string{"npc_1_Chat 1", "Quest Q001 start", ""},
		},
		{
			name: "blank prefix after -h is dropped",
			html: `<a action="bypass -h  $x">x</a>`,
			deny: []string{"npc_1_Chat 1", "Quest Q001 start"},
		},
		{
			name: "empty prefix without -h is dropped",
			html: `<a action="bypass $x">x</a>`,
			deny: []string{"npc_1_Chat 1"},
		},
		{
			name:  "-h link with nothing after it is skipped",
			html:  `<a action="bypass -h">x</a><a action="bypass npc_1_Chat 3">y</a>`,
			allow: []string{"npc_1_Chat 3"},
			deny:  []string{"npc_1_Chat 1", "-h", "h"},
		},
		{
			name: "-h link closing the page is skipped",
			html: `<a action="bypass -h"`,
			deny: []string{"npc_1_Chat 1", "-h"},
		},
		{
			name: "unterminated link is ignored",
			html: `<a action="bypass npc_1_Chat 1`,
			deny: []string{"npc_1_Chat 1"},
		},
		{
			name:  "several links",
			html:  `<a action="bypass -h npc_1_Chat 1">a</a><a action="bypass -h Quest Q001 $ev">b</a>`,
			allow: []string{"npc_1_Chat 1", "Quest Q001 start"},
			deny:  []string{"Quest Q002 start", "npc_1_Chat 2"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var w bypassWhitelist
			w.record(`<a action="bypass -h npc_9_Chat 9">earlier</a>`)
			w.record(tc.html)
			if w.allows("npc_9_Chat 9") {
				t.Fatalf("record kept a link of the earlier page")
			}
			for _, cmd := range tc.allow {
				if !w.allows(cmd) {
					t.Errorf("allows(%q) = false, want true", cmd)
				}
			}
			for _, cmd := range tc.deny {
				if w.allows(cmd) {
					t.Errorf("allows(%q) = true, want false", cmd)
				}
			}
		})
	}
}

func TestBypassWhitelistTooLongPageClears(t *testing.T) {
	var w bypassWhitelist
	w.record(serverpackets.NpcHtmlBody(`<a action="bypass -h npc_1_Chat 1">x</a>`))
	if !w.allows("npc_1_Chat 1") {
		t.Fatalf("short page link not recorded")
	}

	long := `<html><body><a action="bypass -h npc_1_Chat 2">x</a><a action="bypass -h npc_1_Link $x">y</a>` +
		strings.Repeat("a", 8192) + `</body></html>`
	w.record(serverpackets.NpcHtmlBody(long))
	for _, cmd := range []string{"npc_1_Chat 1", "npc_1_Chat 2", "npc_1_Link a.htm"} {
		if w.allows(cmd) {
			t.Errorf("allows(%q) = true after a too-long page, want false", cmd)
		}
	}
}
