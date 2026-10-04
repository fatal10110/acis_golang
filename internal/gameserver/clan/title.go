package clan

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// titleGrantMinLevel is the clan level a member needs before it may give
// titles.
const titleGrantMinLevel = 3

// TitleGrant is the outcome of a request to give a title.
type TitleGrant int

// The title request outcomes; the refusals in the order they are checked.
const (
	// TitleSelf lets a noble that named itself take the title, whether or
	// not it is in a clan.
	TitleSelf TitleGrant = iota
	// TitleMember lets the requester give the title to the member named,
	// which may be the requester itself.
	TitleMember
	// TitleInvalid refuses a title holding a character no title may.
	TitleInvalid
	// TitleNotAuthorized refuses a requester outside a clan or without the
	// title privilege.
	TitleNotAuthorized
	// TitleClanLevel refuses a clan below level 3.
	TitleClanLevel
	// TitleNotMember refuses a name no member of the clan carries, matched
	// with case.
	TitleNotMember
)

// GrantTitle judges c's request to give the player named name the title
// title. On TitleMember it returns the clan and the member named; whether
// that member is in the world is the caller's to check.
func (s *Service) GrantTitle(c *player.Character, name, title string) (*Clan, Member, TitleGrant) {
	if !ValidTitle(title) {
		return nil, Member{}, TitleInvalid
	}
	// The name is matched exactly: a character name holds no pattern
	// character, so the reference's pattern match is an equality.
	if c.IsNoble() && name == c.Name {
		return nil, Member{}, TitleSelf
	}
	cl, ok := s.ClanOf(c)
	if !ok || !cl.HasPrivilege(c.ID, PrivManageTitles) {
		return nil, Member{}, TitleNotAuthorized
	}
	if cl.Level() < titleGrantMinLevel {
		return nil, Member{}, TitleClanLevel
	}
	m, ok := cl.MemberByName(name)
	if !ok {
		return nil, Member{}, TitleNotMember
	}
	return cl, m, TitleMember
}

// ValidTitle reports whether title holds only the characters a given
// title may: ASCII letters and digits, the space and !@#$&()-`.+,/". The
// reference's pattern bounds the length with a quantifier that never
// applies, so a title of any length passes; it is cut to 16 characters as
// it is set.
func ValidTitle(title string) bool {
	const punctuation = " !@#$&()-`.+,/\""
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r < 0x80 && strings.IndexByte(punctuation, byte(r)) >= 0:
		default:
			return false
		}
	}
	return true
}
