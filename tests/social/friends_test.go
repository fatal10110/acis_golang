package social

import (
	"fmt"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestFriendInviteAccepted pins RequestFriendInvite and
// RequestAnswerFriendInvite on the accepting path. The invitation finds the
// target by name whatever its case and shows it a FriendAddRequest naming
// the requester; the requester hears nothing yet. Accepting answers each
// side, in this order, with FriendAddRequestResult(1), its system message
// (S1_ADDED_TO_FRIENDS to the requester, S1_JOINED_AS_FRIEND to the target)
// and an L2Friend add row for the other. The friendship is saved with the
// relations, as one row flagged 1.
func TestFriendInviteAccepted(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.alice.Send(encodeFriendInvite("bOBBY"))
	frame := p.bobby.Read()
	assertOpcode(t, frame, serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	if got := wire.NewReader(frame[1:]).ReadString(); got != "Alice" {
		t.Fatalf("FriendAddRequest requester = %q, want Alice", got)
	}
	assertSilent(t, p.alice, "requester while the invitation is pending")

	p.bobby.Send(encodeAnswerFriendInvite(1))
	assertAddResult(t, p.alice.Read(), true)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1AddedToFriends, "Bobby")
	assertL2Friend(t, p.alice.Read(), serverpackets.L2FriendAdd, "Bobby", true, p.bobbyID)
	assertAddResult(t, p.bobby.Read(), true)
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1JoinedAsFriend, "Alice")
	assertL2Friend(t, p.bobby.Read(), serverpackets.L2FriendAdd, "Alice", true, p.aliceID)
	assertSilent(t, p.alice, "requester after the accept")
	assertSilent(t, p.bobby, "target after the accept")

	if !p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("Alice and Bobby are not friends after the accept")
	}
	p.srv.SaveRelations(t)
	if got := relationRow(t, p, p.aliceID, p.bobbyID); got != 1 {
		t.Fatalf("character_relations flags = %d, want 1", got)
	}

	// The invitation is spent: a second answer finds none and answers
	// nothing.
	p.bobby.Send(encodeAnswerFriendInvite(1))
	assertSilent(t, p.bobby, "target answering a spent invitation")
	assertSilent(t, p.alice, "requester after a spent invitation's answer")
}

// TestFriendInviteDeclined answers the requester alone, with
// FriendAddRequestResult(0), and makes no friends.
func TestFriendInviteDeclined(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	p.bobby.Send(encodeAnswerFriendInvite(0))
	assertAddResult(t, p.alice.Read(), false)
	assertSilent(t, p.alice, "requester after the decline")
	assertSilent(t, p.bobby, "target after declining")
	if p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("a declined invitation made friends")
	}
}

// TestFriendInviteRefusals pins every refusal of RequestFriendInvite, in
// the order they are checked: each answers the requester with its system
// message and FriendAddRequestResult(0), and the target hears nothing. The
// messages naming the target name it as typed.
func TestFriendInviteRefusals(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, p *pair)
		invite string
		want   func(t *testing.T, frame []byte)
	}{
		{
			name:   "nobody of that name online",
			invite: "Nobody",
			want: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageTargetNotFound)
			},
		},
		{
			name:   "oneself",
			invite: "alice",
			want: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageCannotAddYourselfToFriendsList)
			},
		},
		{
			name: "target blocking everything",
			setup: func(t *testing.T, p *pair) {
				p.bobby.Send(encodeBlock(clientpackets.BlockAll, ""))
				drainUntilQuiet(t, p.bobby)
			},
			invite: "bobby",
			want: func(t *testing.T, f []byte) {
				assertSystemMessageText(t, f, serverpackets.SystemMessageS1BlockedEverything, "bobby")
			},
		},
		{
			name: "requester on the target's block list",
			setup: func(t *testing.T, p *pair) {
				p.srv.Relations.Block(p.bobbyID, p.aliceID)
			},
			invite: "bobby",
			want: func(t *testing.T, f []byte) {
				assertSystemMessageText(t, f, serverpackets.SystemMessageS1HasAddedYouToIgnoreList2, "bobby")
			},
		},
		{
			name: "already friends",
			setup: func(t *testing.T, p *pair) {
				p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
			},
			invite: "Bobby",
			want: func(t *testing.T, f []byte) {
				assertSystemMessageText(t, f, serverpackets.SystemMessageS1AlreadyInFriendsList, "Bobby")
			},
		},
		{
			name: "target holding another invitation",
			setup: func(t *testing.T, p *pair) {
				carol, _ := p.third(t, "player3", "Carol")
				carol.Send(encodeFriendInvite("Bobby"))
				assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "Carol's FriendAddRequest")
			},
			invite: "Bobby",
			want: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageWaitingForAnotherReply)
			},
		},
		{
			name: "target waiting on its own invitation",
			setup: func(t *testing.T, p *pair) {
				carol, _ := p.third(t, "player3", "Carol")
				p.bobby.Send(encodeFriendInvite("Carol"))
				assertOpcode(t, carol.Read(), serverpackets.OpcodeFriendAddRequest, "Bobby's FriendAddRequest")
			},
			invite: "Bobby",
			want: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageWaitingForAnotherReply)
			},
		},
		{
			name: "target holding a trade request",
			setup: func(t *testing.T, p *pair) {
				carol, _ := p.third(t, "player3", "Carol")
				carol.Send(encodeTradeRequest(p.bobbyID))
				assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeSendTradeRequest, "Carol's SendTradeRequest")
				drainUntilQuiet(t, carol)
			},
			invite: "Bobby",
			want: func(t *testing.T, f []byte) {
				assertStaticSystemMessage(t, f, serverpackets.SystemMessageWaitingForAnotherReply)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := bootPair(t)
			p.enterAll(t)
			if tc.setup != nil {
				tc.setup(t, p)
			}

			p.alice.Send(encodeFriendInvite(tc.invite))
			tc.want(t, p.alice.Read())
			assertAddResult(t, p.alice.Read(), false)
			assertSilent(t, p.alice, "requester after the refusal")
			assertSilent(t, p.bobby, "target after the refusal")
		})
	}
}

// TestFriendInviteToGMRefused refuses a non-GM's invitation to a GM with
// THE_PLAYER_IS_REJECTING_FRIEND_INVITATIONS, while a GM may invite a GM.
func TestFriendInviteToGMRefused(t *testing.T) {
	levels, err := admin.NewData([]admin.AccessLevel{
		{Level: 0, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true},
		{Level: 7, Name: "GM", NameColor: "FFFFFF", TitleColor: "FFFF77", IsGM: true, AllowTransaction: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	p := bootPair(t, gameservertest.WithAdmin(levels))
	p.setAccessLevel(t, p.bobbyID, 7)
	p.enterAll(t)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessagePlayerIsRejectingFriendInvitations)
	assertAddResult(t, p.alice.Read(), false)
	assertSilent(t, p.bobby, "GM after a refused invitation")

	p.bobby.Send(encodeFriendInvite("Alice"))
	assertOpcode(t, p.alice.Read(), serverpackets.OpcodeFriendAddRequest, "a GM's FriendAddRequest")
}

// TestFriendInviteExpires lets an invitation outlive its 15 seconds: the
// late answer finds none and nobody hears anything, and the target is free
// for a new invitation.
func TestFriendInviteExpires(t *testing.T) {
	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	now := func() time.Time { return time.Unix(0, clock.Load()) }
	p := bootPair(t, gameservertest.WithTradeClock(now))
	p.enterAll(t)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	clock.Add(int64(trade.RequestTimeout))

	p.bobby.Send(encodeAnswerFriendInvite(1))
	assertSilent(t, p.bobby, "target answering an expired invitation")
	assertSilent(t, p.alice, "requester after an expired invitation's answer")
	if p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("an expired invitation made friends")
	}

	carol, _ := p.third(t, "player3", "Carol")
	carol.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest after expiry")
}

// TestFriendInviteAnswerEndsRequesterOtherInvitations pins the shared
// invitation clock: the first answer to any of one requester's invitations
// ends the others, whose late answers then find nothing.
func TestFriendInviteAnswerEndsRequesterOtherInvitations(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)
	carol, _ := p.third(t, "player3", "Carol")

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest to Bobby")
	p.alice.Send(encodeFriendInvite("Carol"))
	assertOpcode(t, carol.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest to Carol")

	p.bobby.Send(encodeAnswerFriendInvite(0))
	assertAddResult(t, p.alice.Read(), false)

	carol.Send(encodeAnswerFriendInvite(1))
	assertSilent(t, carol, "Carol answering an ended invitation")
	assertSilent(t, p.alice, "requester after Carol's late answer")
}

// TestFriendInviteRequesterRelogged pins an invitation whose requester
// restarted before the answer: the invitation still stands, and accepting
// makes the two friends, but the outcome belongs to the login that asked.
// The new login hears nothing, and the target's add row shows the
// requester offline.
func TestFriendInviteRequesterRelogged(t *testing.T) {
	p := bootPair(t)
	p.enterAll(t)

	p.alice.Send(encodeFriendInvite("Bobby"))
	assertOpcode(t, p.bobby.Read(), serverpackets.OpcodeFriendAddRequest, "FriendAddRequest")
	p.restart(t, p.alice, p.aliceID)
	startInWorld(t, p.alice)
	drainUntilQuiet(t, p.alice)
	drainUntilQuiet(t, p.bobby)

	p.bobby.Send(encodeAnswerFriendInvite(1))
	assertAddResult(t, p.bobby.Read(), true)
	assertSystemMessageText(t, p.bobby.Read(), serverpackets.SystemMessageS1JoinedAsFriend, "Alice")
	assertL2Friend(t, p.bobby.Read(), serverpackets.L2FriendAdd, "Alice", false, p.aliceID)
	assertSilent(t, p.alice, "relogged requester after the old invitation's accept")
	if !p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) {
		t.Fatal("the accepted invitation made no friends")
	}
}

// TestFriendListAndStatusNotices pins the friend list at login and the
// notices friends get. A player entering with an offline friend gets that
// friend's FriendList row offline (object id 0 in the online slot); when
// the friend enters, the first player gets L2FriendStatus(1) followed by
// FRIEND_S1_HAS_LOGGED_IN, and the entering friend's own FriendList shows
// the first player online. Leaving sends L2FriendStatus(0) alone.
func TestFriendListAndStatusNotices(t *testing.T) {
	p := bootPair(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)

	burst := startInWorld(t, p.alice)
	assertFriendList(t, burst[8], []serverpackets.FriendListEntry{{ObjectID: p.bobbyID, Name: "Bobby"}})
	drainUntilQuiet(t, p.alice)

	burst = startInWorld(t, p.bobby)
	assertFriendList(t, burst[8], []serverpackets.FriendListEntry{{ObjectID: p.aliceID, Name: "Alice", Online: true}})
	frames := drainFrames(t, p.alice)
	i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeL2FriendStatus })
	if i < 0 || i+1 >= len(frames) {
		t.Fatalf("friend got no L2FriendStatus then message at login: %d frames", len(frames))
	}
	assertFriendStatus(t, frames[i], true, "Bobby", p.bobbyID)
	assertSystemMessageText(t, frames[i+1], serverpackets.SystemMessageFriendS1HasLoggedIn, "Bobby")
	drainUntilQuiet(t, p.bobby)

	p.alice.Send(encodeFriendList())
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1Online, "Bobby")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)

	p.bobby.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	p.awaitOffline(t, p.bobbyID)
	frames = drainFrames(t, p.alice)
	i = slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeL2FriendStatus })
	if i < 0 {
		t.Fatal("friend got no L2FriendStatus at logout")
	}
	assertFriendStatus(t, frames[i], false, "Bobby", p.bobbyID)
	if i+1 < len(frames) && frames[i+1][0] == serverpackets.OpcodeSystemMessage {
		t.Fatal("logout notice carries a system message")
	}

	p.alice.Send(encodeFriendList())
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1Offline, "Bobby")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)
}

// TestFriendListOrder pins the order of the login FriendList and of the
// /friendlist lines: the iteration order of the hash table the list is
// gathered into, so by id modulo the table size (the ids here are far below
// 65536, where the hash's high-half fold changes nothing), not by id. The
// table starts at 16 buckets and doubles once it holds more than 12 ids, or
// when a ninth id lands in one bucket.
func TestFriendListOrder(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seeded  int
		friend  func(i int, id int32) bool
		limit   int // stop seeding once this many are friends; 0 never
		buckets int32
	}{
		// Every third of 18 characters: 6 friends across more than one lap
		// of 16 buckets.
		{"sixteen buckets", 18, func(i int, _ int32) bool { return i%3 == 0 }, 0, 16},
		// Every third of 42: 14 friends, past three quarters of 16.
		{"grown by size", 42, func(i int, _ int32) bool { return i%3 == 0 }, 0, 32},
		// Nine ids sharing one bucket of 16 grow the table though only
		// nine are held.
		{"grown by a crowded bucket", 200, func(_ int, id int32) bool { return id%16 == 5 }, 9, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := bootPair(t)
			var ids []int32
			names := map[int32]string{}
			for i := range tc.seeded {
				if tc.limit > 0 && len(ids) == tc.limit {
					break
				}
				name := fmt.Sprintf("Friend%03d", i)
				c := p.srv.SeedCharacterFor(t, "friends", name, 1, 0)
				if tc.friend(i, c.ID) {
					ids = append(ids, c.ID)
					names[c.ID] = name
					p.srv.Relations.AddFriend(p.aliceID, c.ID)
				}
			}
			want := slices.Clone(ids)
			slices.SortStableFunc(want, func(a, b int32) int { return int(a%tc.buckets) - int(b%tc.buckets) })
			if slices.Equal(want, ids) {
				t.Fatalf("friend ids %v already ascend by bucket; the scenario needs ids past one lap", ids)
			}

			burst := startInWorld(t, p.alice)
			entries := make([]serverpackets.FriendListEntry, len(want))
			for i, id := range want {
				entries[i] = serverpackets.FriendListEntry{ObjectID: id, Name: names[id]}
			}
			assertFriendList(t, burst[8], entries)
			drainUntilQuiet(t, p.alice)

			p.alice.Send(encodeFriendList())
			assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListHeader)
			for _, id := range want {
				assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1Offline, names[id])
			}
			assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)
		})
	}
}

// TestFriendDelete pins RequestFriendDel. Against an online friend: the
// deleter gets S1_HAS_BEEN_DELETED_FROM_YOUR_FRIENDS_LIST with the stored
// name and an L2Friend remove row, the other side only its own remove row.
// Against an offline friend the remove row carries the name as typed and no
// object id. A name that is no friend answers THE_USER_NOT_IN_FRIENDS_LIST.
func TestFriendDelete(t *testing.T) {
	p := bootPair(t)
	carolID := p.srv.SeedCharacterFor(t, "player3", "Carol", 1, 0).ID
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	p.srv.Relations.AddFriend(p.aliceID, carolID)
	p.enterAll(t)

	p.alice.Send(encodeFriendDel("BOBBY"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1DeletedFromFriendsList, "Bobby")
	assertL2Friend(t, p.alice.Read(), serverpackets.L2FriendRemove, "Bobby", true, p.bobbyID)
	assertL2Friend(t, p.bobby.Read(), serverpackets.L2FriendRemove, "Alice", true, p.aliceID)
	assertSilent(t, p.alice, "deleter after deleting an online friend")
	assertSilent(t, p.bobby, "deleted friend")

	p.alice.Send(encodeFriendDel("carol"))
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1DeletedFromFriendsList, "Carol")
	assertL2Friend(t, p.alice.Read(), serverpackets.L2FriendRemove, "carol", false, 0)

	for _, name := range []string{"Bobby", "Nobody"} {
		p.alice.Send(encodeFriendDel(name))
		assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageUserNotInFriendsList)
		assertSilent(t, p.alice, "deleter after a refused delete of "+name)
	}

	if p.srv.Relations.AreFriends(p.aliceID, p.bobbyID) || p.srv.Relations.AreFriends(p.aliceID, carolID) {
		t.Fatal("friends remain after their deletion")
	}
	p.srv.SaveRelations(t)
	if got := relationRow(t, p, p.aliceID, p.bobbyID); got != -1 {
		t.Fatalf("character_relations row after deletion = %d, want none", got)
	}
}

func assertFriendList(t *testing.T, frame []byte, want []serverpackets.FriendListEntry) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeFriendList, "FriendList")
	r := wire.NewReader(frame[1:])
	if n := r.ReadInt32(); n != int32(len(want)) {
		t.Fatalf("FriendList count = %d, want %d", n, len(want))
	}
	for i, w := range want {
		id, name, online, onlineID := r.ReadInt32(), r.ReadString(), r.ReadInt32(), r.ReadInt32()
		wantOnline, wantOnlineID := int32(0), int32(0)
		if w.Online {
			wantOnline, wantOnlineID = 1, w.ObjectID
		}
		if id != w.ObjectID || name != w.Name || online != wantOnline || onlineID != wantOnlineID {
			t.Fatalf("FriendList row %d = (%d, %q, %d, %d), want (%d, %q, %d, %d)", i, id, name, online, onlineID, w.ObjectID, w.Name, wantOnline, wantOnlineID)
		}
	}
}

func assertFriendStatus(t *testing.T, frame []byte, online bool, name string, objectID int32) {
	t.Helper()
	assertOpcode(t, frame, serverpackets.OpcodeL2FriendStatus, "L2FriendStatus")
	r := wire.NewReader(frame[1:])
	gotOnline, gotName, gotID := r.ReadInt32(), r.ReadString(), r.ReadInt32()
	wantOnline := int32(0)
	if online {
		wantOnline = 1
	}
	if gotOnline != wantOnline || gotName != name || gotID != objectID {
		t.Fatalf("L2FriendStatus = (%d, %q, %d), want (%d, %q, %d)", gotOnline, gotName, gotID, wantOnline, name, objectID)
	}
}
