package clan

import (
	"context"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
)

// MembershipRow is the characters-row side of joining or founding a clan.
type MembershipRow struct {
	ObjectID   int32
	ClanID     int32
	Title      string
	PowerGrade int
	PledgeType int
	JoinExpiry int64
}

// RemovalRow is the characters-row side of leaving a clan. An online
// member's row keeps its creation penalty unless it led the clan; an
// offline member's is cleared unless it led the clan, as the two paths of
// the stored behavior do.
type RemovalRow struct {
	ObjectID     int32
	JoinExpiry   int64
	CreateExpiry int64
	// Online selects the online member's columns: its power grade is reset
	// and its creation penalty written as given; the offline path leaves
	// the power grade and clears the sponsor links pointing at the member.
	Online bool
}

// Store writes the clan rows.
type Store interface {
	InsertClan(ctx context.Context, r Row) error
	UpdateClan(ctx context.Context, r Row) error
	UpdateLevel(ctx context.Context, clanID int32, level int) error
	UpdateReputation(ctx context.Context, clanID int32, score int) error
	SetPrivileges(ctx context.Context, clanID int32, rank int, privs int32) error
	SaveMembership(ctx context.Context, r MembershipRow) error
	RemoveMembership(ctx context.Context, r RemovalRow) error
	SetPowerGrade(ctx context.Context, objectID int32, grade int) error
	// UpdateCrest stores one of the clan's own crest id columns: the
	// pledge, large pledge or alliance crest, as typ names it.
	UpdateCrest(ctx context.Context, clanID int32, typ datacache.CrestType, crestID int32) error
}

// Writer runs a store write later, on ownerID's lane, so writes for one
// owner keep their order and none blocks the caller.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}
