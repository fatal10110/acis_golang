package party

import (
	"testing"
	"time"
)

type invitee struct {
	id                                       int32
	cursed, invisible, detached, jailed, oly bool
}

func (i *invitee) ObjectID() int32            { return i.id }
func (i *invitee) CursedWeaponEquipped() bool { return i.cursed }
func (i *invitee) Invisible() bool            { return i.invisible }
func (i *invitee) ClientDetached() bool       { return i.detached }
func (i *invitee) Jailed() bool               { return i.jailed }
func (i *invitee) OlympiadMode() bool         { return i.oly }

// The invitation refusals keep their order: each case sets every later
// cause too, so a refusal checked out of order shows.
func TestCheckInviteOrder(t *testing.T) {
	r := NewRegistry[*member](func() time.Time { return time.Unix(100, 0) })
	ms := newMembers(3)
	formParty(t, r, ms[1], ms[2])
	all := func(id int32) *invitee {
		return &invitee{id: id, cursed: true, invisible: true, detached: true, jailed: true, oly: true}
	}
	tests := []struct {
		name            string
		inviter, target *invitee
		want            InviteRefusal
	}{
		{"self", &invitee{id: 10}, &invitee{id: 10}, InviteWrongTarget},
		{"inviter cursed", &invitee{id: 10, cursed: true}, all(ms[1].id), InviteWrongTarget},
		{"target cursed", &invitee{id: 10}, all(ms[1].id), InviteWrongTarget},
		{"target invisible", &invitee{id: 10}, &invitee{id: ms[1].id, invisible: true, detached: true, jailed: true, oly: true}, InviteWrongTarget},
		{"target in party", &invitee{id: 10}, &invitee{id: ms[1].id, detached: true, jailed: true, oly: true}, InviteTargetInParty},
		{"target detached", &invitee{id: 10, jailed: true}, &invitee{id: 20, detached: true, jailed: true, oly: true}, InviteTargetDetached},
		{"target jailed", &invitee{id: 10}, &invitee{id: 20, jailed: true, oly: true}, InviteJailed},
		{"inviter jailed", &invitee{id: 10, jailed: true, oly: true}, &invitee{id: 20}, InviteJailed},
		{"target in olympiad", &invitee{id: 10}, &invitee{id: 20, oly: true}, InviteOlympiad},
		{"inviter in olympiad", &invitee{id: 10, oly: true}, &invitee{id: 20}, InviteOlympiad},
		{"allowed", &invitee{id: 10}, &invitee{id: 20}, InviteAllowed},
	}
	for _, tt := range tests {
		if got := r.CheckInvite(tt.inviter, tt.target); got != tt.want {
			t.Errorf("%s: CheckInvite = %v, want %v", tt.name, got, tt.want)
		}
	}
}

type former bool

func (f former) HasSkill(id int) bool { return bool(f) && id == clanImperiumSkillID }

// The Strategy Guide is asked for, and so paid, only by a leader of a
// clan of level 5 or more without Clan Imperium.
func TestFormAuthority(t *testing.T) {
	tests := []struct {
		name      string
		imperium  bool
		level     int
		held      bool
		want      ChannelAuthority
		wantAsked bool
	}{
		{"no clan led", true, 0, true, ChannelNotClanLeader, false},
		{"clan level 4", true, 4, true, ChannelNotClanLeader, false},
		{"imperium", true, 5, true, ChannelAuthorized, false},
		{"guide", false, 5, true, ChannelAuthorized, true},
		{"neither", false, 8, false, ChannelNoStrategyGuide, true},
	}
	for _, tt := range tests {
		var asked int32
		got := FormAuthority(former(tt.imperium), tt.level, func(itemID int32) bool {
			asked = itemID
			return tt.held
		})
		if got != tt.want {
			t.Errorf("%s: FormAuthority = %v, want %v", tt.name, got, tt.want)
		}
		if wantID := map[bool]int32{true: strategyGuideItemID}[tt.wantAsked]; asked != wantID {
			t.Errorf("%s: guide asked for item %d, want %d", tt.name, asked, wantID)
		}
	}
}
