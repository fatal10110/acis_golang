package manager

import (
	"fmt"
	"regexp"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// DefaultDeleteAfter is the grace period between scheduling a character for
// deletion and it actually being purged, matching the shipped default.
const DefaultDeleteAfter = 7 * 24 * time.Hour

// MaxCharactersPerAccount is how many characters one account may hold.
const MaxCharactersPerAccount = 7

// validCharacterName matches the client's own name charset check: 1-16
// ASCII letters or digits, nothing else.
var validCharacterName = regexp.MustCompile(`^[A-Za-z0-9]{1,16}$`)

// CreateOutcome reports why Create did not produce a character, so a caller
// can translate it into whatever client-facing reason it needs. CreateOK
// means a character was produced.
type CreateOutcome int

const (
	CreateOK CreateOutcome = iota
	// CreateRejected covers every appearance/profession validation failure
	// that has no more specific outcome below (bad race/face/hairstyle/hair
	// color, or a profession id that isn't a selectable root profession).
	CreateRejected
	CreateTooManyCharacters
	CreateNameTaken
	CreateInvalidName
)

// CreateRequest is the client-supplied choice for a new character.
// Appearance fields are validated by Create; nothing here is trusted
// as-is.
type CreateRequest struct {
	Name    string
	ClassID int
	// Race is validated for range but never stored: a character's actual
	// race always follows from ClassID (see player.ClassRace). The wire
	// protocol carries it anyway, so an out-of-range value is still rejected
	// as the client-facing creation check requires.
	Race                       int
	Sex                        player.Sex
	HairStyle, HairColor, Face byte
}

// idAllocator hands out unique persistent-object ids. Roster needs only
// this much of the server's id allocator to number a new character or item.
type idAllocator interface {
	NextID() (int32, error)
}

// characterStore is the persistence Roster needs for the characters table.
// Satisfied by *sql.CharacterStore.
type characterStore interface {
	Create(c *player.Character) error
	ListByAccount(accountName string) ([]*player.Character, error)
	CountByAccount(accountName string) (int, error)
	NameTaken(name string) (bool, error)
	SetDeleteAt(objectID int32, at int64) error
	Delete(objectID int32) (bool, error)
}

// itemStore is the persistence Roster needs for the items table. Satisfied
// by *sql.ItemStore.
type itemStore interface {
	Create(ownerID int32, inst item.Instance) error
	DeleteByOwner(ownerID int32) (int64, error)
}

// Roster creates, lists, deletes and restores the characters on an
// account, keeping the characters and items tables consistent with each
// other. A Roster holds no mutable state of its own beyond its
// dependencies, so it is safe for concurrent use exactly to the extent its
// stores and id allocator are.
type Roster struct {
	characters characterStore
	items      itemStore
	templates  *player.TemplateTable
	itemTable  *item.Table
	ids        idAllocator

	deleteAfter time.Duration
	now         func() time.Time
}

// NewRoster returns a Roster backed by the given stores and lookup tables.
// deleteAfter is the grace period MarkForDeletion schedules (see
// DefaultDeleteAfter); a zero value means deletion is immediate. now
// defaults to time.Now when nil.
func NewRoster(characters characterStore, items itemStore, templates *player.TemplateTable, itemTable *item.Table, ids idAllocator, deleteAfter time.Duration, now func() time.Time) *Roster {
	if now == nil {
		now = time.Now
	}
	return &Roster{
		characters:  characters,
		items:       items,
		templates:   templates,
		itemTable:   itemTable,
		ids:         ids,
		deleteAfter: deleteAfter,
		now:         now,
	}
}

// Create validates req and, if it passes, allocates a new character owned
// by accountName, persists it, and grants its profession's starter items.
// A caller should check err first: only when err is nil does the returned
// CreateOutcome distinguish success from an expected, client-triggerable
// rejection (a fault always comes back as err, never as CreateRejected).
//
// A character row and its granted items are written as separate
// statements, not one transaction: an error partway through Create can
// leave a character row with only some of its starter items granted.
// Reconciling that is a job for whatever boot or maintenance pass
// eventually audits character/item consistency, not for Create to guess
// at by retrying or rolling back.
//
// One validation is intentionally not implemented yet: rejecting a name
// that collides with an NPC's name. That check reads a data table this
// port hasn't built yet; a name collision with an NPC is merely cosmetic
// (chat/targeting ambiguity), not a correctness risk, so it is deferred
// rather than pulled in early.
func (r *Roster) Create(accountName string, req CreateRequest) (*player.Character, CreateOutcome, error) {
	if req.Race < 0 || req.Race > 4 {
		return nil, CreateRejected, nil
	}
	if req.Face > 2 {
		return nil, CreateRejected, nil
	}
	if req.HairStyle > hairStyleLimit(req.Sex) {
		return nil, CreateRejected, nil
	}
	if req.HairColor > 3 {
		return nil, CreateRejected, nil
	}
	if !validCharacterName.MatchString(req.Name) {
		return nil, CreateInvalidName, nil
	}

	count, err := r.characters.CountByAccount(accountName)
	if err != nil {
		return nil, CreateRejected, err
	}
	if count >= MaxCharactersPerAccount {
		return nil, CreateTooManyCharacters, nil
	}

	taken, err := r.characters.NameTaken(req.Name)
	if err != nil {
		return nil, CreateRejected, err
	}
	if taken {
		return nil, CreateNameTaken, nil
	}

	tmpl, ok := r.templates.Get(req.ClassID)
	if !ok || tmpl.BaseLevel > 1 {
		// Only a root profession (the 9 starting classes) may be chosen at
		// creation; every upgrade is reached by playing, not by picking it
		// up front.
		return nil, CreateRejected, nil
	}

	id, err := r.ids.NextID()
	if err != nil {
		return nil, CreateRejected, fmt.Errorf("player roster: create %q: %w", req.Name, err)
	}

	c, err := player.NewCharacter(id, tmpl, accountName, req.Name, req.HairStyle, req.HairColor, req.Face, req.Sex)
	if err != nil {
		return nil, CreateRejected, fmt.Errorf("player roster: create %q: %w", req.Name, err)
	}

	if err := r.characters.Create(c); err != nil {
		return nil, CreateRejected, err
	}

	for _, grant := range tmpl.Items {
		if err := r.grantItem(c.ObjectID, grant); err != nil {
			return nil, CreateRejected, err
		}
	}

	return c, CreateOK, nil
}

func (r *Roster) grantItem(ownerID int32, grant player.StarterItem) error {
	tmpl, ok := r.itemTable.Get(int32(grant.ItemID))
	if !ok {
		return fmt.Errorf("player roster: starter item %d has no template", grant.ItemID)
	}

	itemID, err := r.ids.NextID()
	if err != nil {
		return fmt.Errorf("player roster: grant item %d: %w", grant.ItemID, err)
	}

	inst := item.NewStackOrEquip(itemID, tmpl, grant.Count, grant.Equipped)
	if err := r.items.Create(ownerID, inst); err != nil {
		return err
	}
	return nil
}

// hairStyleLimit returns the highest hairstyle index sex may choose: male
// character models expose fewer hairstyles than female ones.
func hairStyleLimit(sex player.Sex) byte {
	if sex == player.SexMale {
		return 4
	}
	return 6
}

// List returns the characters on accountName, purging (and excluding) any
// whose scheduled deletion deadline has already passed.
func (r *Roster) List(accountName string) ([]*player.Character, error) {
	chars, err := r.characters.ListByAccount(accountName)
	if err != nil {
		return nil, err
	}

	now := r.now().UnixMilli()
	live := chars[:0]
	for _, c := range chars {
		if c.DeleteAt > 0 && now > c.DeleteAt {
			if err := r.purge(c.ObjectID); err != nil {
				return nil, err
			}
			continue
		}
		live = append(live, c)
	}
	return live, nil
}

func (r *Roster) purge(objectID int32) error {
	if _, err := r.characters.Delete(objectID); err != nil {
		return err
	}
	if _, err := r.items.DeleteByOwner(objectID); err != nil {
		return err
	}
	return nil
}

// MarkForDeletion schedules objectID for deletion after the Roster's grace
// period, or purges it immediately when that period is zero.
//
// Deletion should also be blocked for a clan's leader or member; without a
// clan system yet, every character is treated as clan-free and deletion
// always proceeds.
func (r *Roster) MarkForDeletion(objectID int32) error {
	if r.deleteAfter <= 0 {
		return r.purge(objectID)
	}
	return r.characters.SetDeleteAt(objectID, r.now().Add(r.deleteAfter).UnixMilli())
}

// Restore clears objectID's scheduled deletion.
func (r *Roster) Restore(objectID int32) error {
	return r.characters.SetDeleteAt(objectID, 0)
}
