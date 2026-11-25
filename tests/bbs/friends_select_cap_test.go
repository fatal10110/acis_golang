package bbs

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFriendsBoardSelectOnlyOwnList pins #5102: a friends or block board
// select picks only an id on the player's own list and never more ids than
// that list holds; any other select shows the page as it was, naming
// nothing of the id.
func TestFriendsBoardSelectOnlyOwnList(t *testing.T) {
	p := bootPair(t, gameservertest.WithCommunityBoard(boardOn))
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	daveID := p.srv.SeedCharacterFor(t, "player4", "Dave", 1, 0).ID
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.srv.Relations.AddFriend(p.aliceID, carolID)
	p.srv.Relations.Block(p.aliceID, carolID)
	p.enterAll(t)

	list := pageOf(t, command(t, p.alice, "_friendlist_0_"))
	for _, id := range []int32{daveID, p.aliceID, 0, -1} {
		if got := pageOf(t, command(t, p.alice, "_friend;select;"+strconv.Itoa(int(id)))); got != list {
			t.Fatalf("friends list after selecting %d, not a friend = %q, want %q", id, got, list)
		}
	}

	// Bobby picked, then dropped from the list: the stale pick fills the
	// one-friend list, so Carol cannot be picked until Bobby is deselected.
	command(t, p.alice, "_friend;select;"+strconv.Itoa(int(p.bobbyID)))
	p.srv.Relations.RemoveFriend(p.aliceID, p.bobbyID)
	carolRow := contact("_friend", "select", "[Select]", carolID, "Carol", false)
	bobbyPicked := contact("_friend", "deselect", "[Deselect]", p.bobbyID, "Bobby", true)
	if got, want := pageOf(t, command(t, p.alice, "_friend;select;"+strconv.Itoa(int(carolID)))), "FRIENDS list="+carolRow+" picked="+bobbyPicked+" del=\n"; got != want {
		t.Fatalf("friends list selecting past the cap = %q, want %q", got, want)
	}
	command(t, p.alice, "_friend;deselect;"+strconv.Itoa(int(p.bobbyID)))
	carolPicked := contact("_friend", "deselect", "[Deselect]", carolID, "Carol", false)
	if got, want := pageOf(t, command(t, p.alice, "_friend;select;"+strconv.Itoa(int(carolID)))), "FRIENDS list= picked="+carolPicked+" del=\n"; got != want {
		t.Fatalf("friends list with Carol picked = %q, want %q", got, want)
	}

	blockRow := contact("_block", "select", "[Select]", carolID, "Carol", false)
	if got, want := pageOf(t, command(t, p.alice, "_block;select;"+strconv.Itoa(int(daveID)))), "BLOCKS list="+blockRow+" picked= del=\n"; got != want {
		t.Fatalf("block list after selecting Dave, not blocked = %q, want %q", got, want)
	}
	blockPicked := contact("_block", "deselect", "[Deselect]", carolID, "Carol", false)
	if got, want := pageOf(t, command(t, p.alice, "_block;select;"+strconv.Itoa(int(carolID)))), "BLOCKS list= picked="+blockPicked+" del=\n"; got != want {
		t.Fatalf("block list with Carol picked = %q, want %q", got, want)
	}
	if got := pageOf(t, command(t, p.alice, "_friendlist_0_")) + pageOf(t, command(t, p.alice, "_blocklist_0_")); strings.Contains(got, "Dave") {
		t.Fatalf("board pages name Dave, on neither list: %q", got)
	}
}

// TestCharacterNamesPastPlaceholderLimit pins the names lookup a board list
// renders through (#5102): a list longer than one query can bind still
// names every character on it.
func TestCharacterNamesPastPlaceholderLimit(t *testing.T) {
	p := bootPair(t)
	ids := make([]int32, 0, 70_000)
	for i := range int32(70_000) {
		ids = append(ids, 1_900_000_000+i)
	}
	ids = append(ids, p.bobbyID)
	ids = append([]int32{p.aliceID}, ids...)
	names, err := p.srv.Chars.Names(context.Background(), ids)
	if err != nil {
		t.Fatalf("names of %d ids: %v", len(ids), err)
	}
	if len(names) != 2 || names[p.aliceID] != "Alice" || names[p.bobbyID] != "Bobby" {
		t.Fatalf("names of %d ids = %v, want Alice and Bobby", len(ids), names)
	}
}
