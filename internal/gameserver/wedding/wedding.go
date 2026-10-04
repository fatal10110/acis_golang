// Package wedding is the wedding manager NPC: the couples it marries, the
// marriage request one player sends another through it, and its dialog.
package wedding

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Config is the npcs.properties wedding section.
type Config struct {
	// Price is WeddingPrice: the adena each spouse pays.
	Price int
	// SameSex is WeddingAllowSameSex.
	SameSex bool
	// FormalWear is WeddingFormalWear: both spouses must wear formal wear.
	FormalWear bool
}

// DefaultConfig is the shipped wedding section.
func DefaultConfig() Config { return Config{Price: 1_000_000, FormalWear: true} }

// The wedding manager's pages.
const (
	PageStart          = "data/html/mods/wedding/start.htm"
	PageMarried        = "data/html/mods/wedding/start2.htm"
	PageWaiting        = "data/html/mods/wedding/waitforpartner.htm"
	PageNotFound       = "data/html/mods/wedding/notfound.htm"
	PageWrongTarget    = "data/html/mods/wedding/error_wrongtarget.htm"
	PageSex            = "data/html/mods/wedding/error_sex.htm"
	PageFriendList     = "data/html/mods/wedding/error_friendlist.htm"
	PageAlreadyMarried = "data/html/mods/wedding/error_alreadymarried.htm"
	PageNoFormal       = "data/html/mods/wedding/error_noformal.htm"
	PageAdena          = "data/html/mods/wedding/error_adena.htm"
)

// The skills a wedding shows on each spouse: the wedding march, then the
// fireworks.
const (
	MarchSkillID     = 2230
	FireworksSkillID = 2025
)

// Couple is one mods_wedding row: the couple's id, the player who asked
// and the player who accepted.
type Couple struct {
	ID          int32
	RequesterID int32
	PartnerID   int32
}

// IDs numbers new couples from the object id space.
type IDs interface {
	NextID() (int32, error)
	ReleaseID(id int32)
}

// Spouse is a player as the wedding manager reads it and marks it while a
// marriage request is pending.
type Spouse interface {
	ObjectID() int32
	Female() bool
	Adena() int
	WearingFormalWear() bool
	// UnderMarryRequest reports a request the player sent or received
	// that is not answered yet.
	UnderMarryRequest() bool
	SetUnderMarryRequest(bool)
	// MarryRequesterID is the player whose request this one was asked to
	// answer, 0 when none is waiting.
	MarryRequesterID() int32
	SetMarryRequesterID(int32)
	// Pay takes count adena from the player and names the amount spent.
	// It takes and says nothing, reporting false, when the player holds
	// less.
	Pay(count int) bool
	// Refund gives back count adena Pay took.
	Refund(count int)
}

// Manager holds every couple and runs the marriage requests. Its methods
// are safe for concurrent use; the request marks it sets on players change
// only under its lock.
type Manager struct {
	cfg Config
	ids IDs

	mu       sync.Mutex
	couples  map[int32]Couple
	byPlayer map[int32]int32
}

// NewManager returns a Manager holding couples, numbering new ones from
// ids. A player stored in more than one couple belongs to the one with
// the lowest id. With nil ids no new couple can be made.
func NewManager(cfg Config, ids IDs, couples []Couple) *Manager {
	m := &Manager{cfg: cfg, ids: ids, couples: make(map[int32]Couple, len(couples)), byPlayer: make(map[int32]int32, 2*len(couples))}
	sorted := slices.Clone(couples)
	slices.SortFunc(sorted, func(a, b Couple) int { return cmp.Compare(a.ID, b.ID) })
	for _, c := range sorted {
		m.couples[c.ID] = c
		m.index(c)
	}
	return m
}

// index records c as the couple of its two players that have none yet.
func (m *Manager) index(c Couple) {
	for _, id := range []int32{c.RequesterID, c.PartnerID} {
		if _, ok := m.byPlayer[id]; !ok {
			m.byPlayer[id] = c.ID
		}
	}
}

// Config returns the wedding settings.
func (m *Manager) Config() Config { return m.cfg }

// Couples returns every couple, by id.
func (m *Manager) Couples() []Couple {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sortedLocked()
}

// sortedLocked returns every couple, by id. m.mu must be held.
func (m *Manager) sortedLocked() []Couple {
	out := make([]Couple, 0, len(m.couples))
	for _, c := range m.couples {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Couple) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

// CoupleID returns the couple player belongs to, 0 when it is not married.
func (m *Manager) CoupleID(player int32) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byPlayer[player]
}

// PartnerID returns the spouse of player, 0 when it is not married.
func (m *Manager) PartnerID(player int32) int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.couples[m.byPlayer[player]]
	if !ok {
		return 0
	}
	if c.RequesterID == player {
		return c.PartnerID
	}
	return c.RequesterID
}

// Greeting is the page an interact opens for s: the married menu, the
// pending request page, or the request form.
func (m *Manager) Greeting(s Spouse) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.byPlayer[s.ObjectID()] > 0:
		return PageMarried
	case s.UnderMarryRequest():
		return PageWaiting
	default:
		return PageStart
	}
}

// Ask has requester ask partner to marry. It returns the refusal page, or
// "" when the request went out: both players are then under the request
// and partner waits on requester's answer. The checks run in order: asking
// oneself, the same sex (unless allowed), the friend list (friends tells
// whether they are friends), a partner already married, formal wear (when
// required) on either, and the price on either.
func (m *Manager) Ask(requester, partner Spouse, friends bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case partner.ObjectID() == requester.ObjectID():
		return PageWrongTarget
	case !m.cfg.SameSex && partner.Female() == requester.Female():
		return PageSex
	case !friends:
		return PageFriendList
	case m.byPlayer[partner.ObjectID()] > 0:
		return PageAlreadyMarried
	case m.cfg.FormalWear && (!requester.WearingFormalWear() || !partner.WearingFormalWear()):
		return PageNoFormal
	case requester.Adena() < m.cfg.Price || partner.Adena() < m.cfg.Price:
		return PageAdena
	}
	requester.SetUnderMarryRequest(true)
	partner.SetUnderMarryRequest(true)
	partner.SetMarryRequesterID(requester.ObjectID())
	return ""
}

// AnswerOutcome is what a player's answer to a marriage request did.
type AnswerOutcome int

const (
	// AnswerIgnored found no request to answer, or its requester offline:
	// nothing changed.
	AnswerIgnored AnswerOutcome = iota
	// AnswerDeclined ended the request unmarried.
	AnswerDeclined
	// AnswerMarried made the couple once each spouse paid the price.
	AnswerMarried
	// AnswerUnpaid ended the request unmarried because a spouse can no
	// longer pay the price.
	AnswerUnpaid
	// AnswerVoid ended the request unmarried because a spouse married
	// someone else in the meantime, or no couple id was left.
	AnswerVoid
)

// Answer is the result of a player's answer to a marriage request.
type Answer struct {
	Outcome AnswerOutcome
	// Requester is the player who asked.
	Requester Spouse
	// RequesterShort and PartnerShort name who could not pay, for
	// AnswerUnpaid.
	RequesterShort, PartnerShort bool
}

// Answer resolves partner's answer to the request it was asked to answer;
// accepted is a yes. online finds the requester among the players online.
// Every outcome but AnswerIgnored ends the request for both players.
func (m *Manager) Answer(partner Spouse, accepted bool, online func(int32) (Spouse, bool)) Answer {
	m.mu.Lock()
	defer m.mu.Unlock()
	requesterID := partner.MarryRequesterID()
	if !partner.UnderMarryRequest() || requesterID == 0 {
		return Answer{}
	}
	requester, ok := online(requesterID)
	if !ok {
		return Answer{}
	}
	out := Answer{Outcome: AnswerDeclined, Requester: requester}
	if accepted {
		out = m.marry(requester, partner)
	}
	requester.SetUnderMarryRequest(false)
	partner.SetUnderMarryRequest(false)
	partner.SetMarryRequesterID(0)
	return out
}

// marry has requester and partner each pay the price, then makes their
// couple, unless either married someone else since the request or can no
// longer pay. The payment is taken before the couple exists, so a spouse
// whose adena leaves between the check and the payment leaves both
// unmarried and, once the other's payment is refunded, neither paying.
func (m *Manager) marry(requester, partner Spouse) Answer {
	out := Answer{Outcome: AnswerVoid, Requester: requester}
	if m.byPlayer[requester.ObjectID()] > 0 || m.byPlayer[partner.ObjectID()] > 0 || m.ids == nil {
		return out
	}
	out.RequesterShort = requester.Adena() < m.cfg.Price
	out.PartnerShort = partner.Adena() < m.cfg.Price
	if out.RequesterShort || out.PartnerShort {
		out.Outcome = AnswerUnpaid
		return out
	}
	id, err := m.ids.NextID()
	if err != nil {
		return out
	}
	if !requester.Pay(m.cfg.Price) {
		m.ids.ReleaseID(id)
		out.Outcome, out.RequesterShort = AnswerUnpaid, true
		return out
	}
	if !partner.Pay(m.cfg.Price) {
		requester.Refund(m.cfg.Price)
		m.ids.ReleaseID(id)
		out.Outcome, out.PartnerShort = AnswerUnpaid, true
		return out
	}
	c := Couple{ID: id, RequesterID: requester.ObjectID(), PartnerID: partner.ObjectID()}
	m.couples[id] = c
	m.index(c)
	out.Outcome = AnswerMarried
	return out
}

// Divorce dissolves the couple player belongs to and frees its id. It
// reports the couple, and false when player was not married.
func (m *Manager) Divorce(player int32) (Couple, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.couples[m.byPlayer[player]]
	if !ok {
		return Couple{}, false
	}
	delete(m.couples, c.ID)
	for _, id := range []int32{c.RequesterID, c.PartnerID} {
		if m.byPlayer[id] == c.ID {
			delete(m.byPlayer, id)
		}
	}
	for _, other := range m.sortedLocked() {
		m.index(other)
	}
	if m.ids != nil {
		m.ids.ReleaseID(c.ID)
	}
	return c, true
}

// Page fills page, a wedding page as set, for the manager npcObjectID:
// its object id, the price and whether formal wear is needed.
func (m *Manager) Page(page string, npcObjectID int32) string {
	page = strings.ReplaceAll(page, "%objectId%", strconv.Itoa(int(npcObjectID)))
	page = strings.ReplaceAll(page, "%adenaCost%", formatNumber(int64(m.cfg.Price)))
	needOrNot := "won't"
	if m.cfg.FormalWear {
		needOrNot = "will"
	}
	return strings.ReplaceAll(page, "%needOrNot%", needOrNot)
}

// formatNumber writes n with a comma between each group of three digits.
func formatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, s = "-", s[1:]
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return sign + s
}

// The notices of the wedding manager's teleport to a spouse.
const (
	NoticePartnerNotFound = "Your partner can't be found."
	NoticePartnerOffline  = "Your partner is not online."
	NoticePartnerStatus   = "Due to the current partner's status, the teleportation failed."
	NoticePartnerInSiege  = "As your partner is in siege, you can't go to him/her."
)

// Destination is a spouse as the teleport to it reads it.
type Destination interface {
	NoSummonFriendZone() bool
	Jailed() bool
	OlympiadMode() bool
	InDuel() bool
	FestivalParticipant() bool
	ObserverMode() bool
}

// TeleportRefusal is the notice refusing a teleport to partner, "" when
// the teleport goes ahead: partner stands where no one may be summoned,
// is jailed, fights in the Olympiad or a duel, takes part in the festival
// or observes.
func TeleportRefusal(partner Destination) string {
	if partner.NoSummonFriendZone() || partner.Jailed() || partner.OlympiadMode() || partner.InDuel() || partner.FestivalParticipant() || partner.ObserverMode() {
		return NoticePartnerStatus
	}
	// ponytail: a partner whose clan owns a castle under siege is refused
	// with NoticePartnerInSiege. No siege can run until the siege engine
	// (#234) exists; it adds the refusal here.
	return ""
}

// The notices of a marriage request's answer and of a divorce.
const (
	NoticeDeclinedByYou     = "You declined your partner's marriage request."
	NoticeDeclinedByPartner = "Your partner declined your marriage request."
	NoticeDivorced          = "You are now divorced."
)

// MarriedNotice congratulates a spouse on marrying partner.
func MarriedNotice(partner string) string {
	return "Congratulations, you are now married with " + partner + " !"
}

// MarriedAnnouncement tells every player online that requester and
// partner married.
func MarriedAnnouncement(requester, partner string) string {
	return "Congratulations to " + requester + " and " + partner + "! They have been married."
}

// RequestText is the marriage request dialog's text naming requester.
func RequestText(requester string) string {
	return requester + " asked you to marry. Do you want to start a new relationship ?"
}
