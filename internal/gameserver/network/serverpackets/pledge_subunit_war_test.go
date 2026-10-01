package serverpackets

import (
	"bytes"
	"strconv"
	"testing"
)

// The fixtures follow PledgeReceiveWarList.java and
// PledgeReceiveSubPledgeCreated.java (reference revision of this branch)
// field by field: 0xfe, the extended opcode as a short, then the fields.
// The war list's count is the stored list's size on tab 0, at most 13 on
// the attacker tab's first page and size % (13 * page) on a later page;
// the attacker tab writes only the names of its page.

func warListHeader(tab, page, count int32) []byte {
	want := []byte{0xfe, 0x3e, 0x00}
	want = appendD(want, tab)
	want = appendD(want, page)
	return appendD(want, count)
}

func warListEntry(want []byte, name string, tab, page int32) []byte {
	want = append(want, encodeUTF16Z(name)...)
	want = appendD(want, tab)
	return appendD(want, page)
}

func TestFramePledgeReceiveWarListDeclaredTab(t *testing.T) {
	// One of the three stored clans no longer exists: the count still
	// reports three, two names follow.
	got := framePayload(t, FramePledgeReceiveWarList(0, 4, 3, []string{"Rivals", "Foes"}))
	want := warListHeader(0, 4, 3)
	want = warListEntry(want, "Rivals", 0, 4)
	want = warListEntry(want, "Foes", 0, 4)
	if !bytes.Equal(got, want) {
		t.Fatalf("war list tab 0 = %x, want %x", got, want)
	}
}

func TestFramePledgeReceiveWarListAttackerPages(t *testing.T) {
	names := make([]string, 15)
	for i := range names {
		names[i] = "Clan" + strconv.Itoa(i)
	}

	got := framePayload(t, FramePledgeReceiveWarList(1, 0, 15, names))
	want := warListHeader(1, 0, 13)
	for _, name := range names[:13] {
		want = warListEntry(want, name, 1, 0)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("attacker tab page 0 = %x, want %x", got, want)
	}

	got = framePayload(t, FramePledgeReceiveWarList(1, 1, 15, names))
	want = warListHeader(1, 1, 2)
	for _, name := range names[13:] {
		want = warListEntry(want, name, 1, 1)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("attacker tab page 1 = %x, want %x", got, want)
	}

	got = framePayload(t, FramePledgeReceiveWarList(2, 0, 4, names[:4]))
	want = warListHeader(2, 0, 4)
	for _, name := range names[:4] {
		want = warListEntry(want, name, 2, 0)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("short attacker tab = %x, want %x", got, want)
	}
}

func TestFramePledgeReceiveSubPledgeCreated(t *testing.T) {
	got := framePayload(t, FramePledgeReceiveSubPledgeCreated(100, "Guards", "Captain"))
	want := []byte{0xfe, 0x3f, 0x00}
	want = appendD(want, 1)
	want = appendD(want, 100)
	want = append(want, encodeUTF16Z("Guards")...)
	want = append(want, encodeUTF16Z("Captain")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("PledgeReceiveSubPledgeCreated = %x, want %x", got, want)
	}
}
