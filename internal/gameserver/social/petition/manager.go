package petition

import (
	"fmt"
	"slices"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"
)

// maxContentLength is the longest petition text, in UTF-16 code units, a
// player may send.
const maxContentLength = 255

// IDs hands out and takes back the object ids petitions are numbered with.
type IDs interface {
	NextID() (int32, error)
	ReleaseID(id int32)
}

// petition is one petition; Manager.mu guards it.
type petition struct {
	Record
	underFeedback bool
}

func (p *petition) responder(id int32) bool { return slices.Contains(p.Responders, id) }

func (p *petition) participant(id int32) bool { return p.Petitioner == id || p.responder(id) }

// Manager holds every petition, in id order. mu guards petitions, order
// and names; a call computes the packets it sends under mu and returns
// them for the caller to send once mu is released.
type Manager struct {
	cfg Config
	ids IDs
	now func() time.Time

	mu        sync.Mutex
	petitions map[int32]*petition
	order     []int32 // petition ids, ascending
	// names is the name each petitioner and responder last went by.
	names map[int32]string
}

// NewManager returns a Manager holding records, names naming their
// petitioners and responders. A nil ids refuses every new petition; a nil
// now reads the wall clock.
func NewManager(cfg Config, ids IDs, now func() time.Time, records []Record, names map[int32]string) *Manager {
	if now == nil {
		now = time.Now
	}
	m := &Manager{cfg: cfg, ids: ids, now: now, petitions: make(map[int32]*petition, len(records)), names: make(map[int32]string, len(names))}
	for id, name := range names {
		m.names[id] = name
	}
	for _, r := range records {
		r.Responders = slices.Clone(r.Responders)
		r.Messages = slices.Clone(r.Messages)
		m.insert(&petition{Record: r})
	}
	return m
}

// Config returns the petition settings.
func (m *Manager) Config() Config { return m.cfg }

// insert files p; m.mu is held or m is not shared yet.
func (m *Manager) insert(p *petition) {
	if _, ok := m.petitions[p.ID]; !ok {
		i, _ := slices.BinarySearch(m.order, p.ID)
		m.order = slices.Insert(m.order, i, p.ID)
	}
	m.petitions[p.ID] = p
}

// each calls fn on every petition in id order until fn returns false;
// m.mu is held.
func (m *Manager) each(fn func(*petition) bool) {
	for _, id := range m.order {
		if !fn(m.petitions[id]) {
			return
		}
	}
}

// find returns the first petition, in id order, match accepts.
func (m *Manager) find(match func(*petition) bool) *petition {
	var found *petition
	m.each(func(p *petition) bool {
		if match(p) {
			found = p
			return false
		}
		return true
	})
	return found
}

func active(p *petition) bool { return p.State == Pending || p.State == Accepted }

// activeCount is how many petitions are active; m.mu is held.
func (m *Manager) activeCount() int {
	n := 0
	for _, p := range m.petitions {
		if active(p) {
			n++
		}
	}
	return n
}

// countOf is how many petitions player sent, cancelled ones aside; m.mu is
// held.
func (m *Manager) countOf(player int32) int {
	n := 0
	for _, p := range m.petitions {
		if p.Petitioner == player && p.State != Cancelled {
			n++
		}
	}
	return n
}

// activeOf reports whether player sent a petition still active; m.mu is
// held.
func (m *Manager) activeOf(player int32) bool {
	return m.find(func(p *petition) bool { return p.Petitioner == player && active(p) }) != nil
}

// inProcess returns the accepted petition player sent or answers; m.mu is
// held.
func (m *Manager) inProcess(player int32) *petition {
	return m.find(func(p *petition) bool { return p.State == Accepted && p.participant(player) })
}

// SubmitResult is how a petition submission ended.
type SubmitResult int

// Submission outcomes, in the order they are checked.
const (
	Submitted SubmitResult = iota
	// SubmitDisabled: petitioning is turned off.
	SubmitDisabled
	// SubmitAlreadyActive: the player has an active petition already.
	SubmitAlreadyActive
	// SubmitServerFull: the server holds as many active petitions as it
	// may.
	SubmitServerFull
	// SubmitPlayerLimit: the player sent as many petitions as it may.
	SubmitPlayerLimit
	// SubmitTooLong: the text is over 255 characters.
	SubmitTooLong
	// SubmitBadType: the type is none the client can name.
	SubmitBadType
	// SubmitNoID: no object id was left to number the petition with.
	SubmitNoID
)

// Submission is a petition submission's outcome. PlayerCount counts the
// player's petitions and ServerCount the server's active ones, each with
// this one; they are set from SubmitServerFull on.
type Submission struct {
	Result      SubmitResult
	ID          int32
	PlayerCount int
	ServerCount int
}

// Submit files player's petition of type typ saying content.
func (m *Manager) Submit(player Person, typ int32, content string) (Submission, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.cfg.Allowed {
		return Submission{Result: SubmitDisabled}, nil
	}
	if m.activeOf(player.ID) {
		return Submission{Result: SubmitAlreadyActive}, nil
	}
	s := Submission{ServerCount: m.activeCount() + 1}
	if s.ServerCount > m.cfg.MaxPending {
		s.Result = SubmitServerFull
		return s, nil
	}
	s.PlayerCount = m.countOf(player.ID) + 1
	if s.PlayerCount > m.cfg.MaxPerPlayer {
		s.Result = SubmitPlayerLimit
		return s, nil
	}
	if len(utf16.Encode([]rune(content))) > maxContentLength {
		s.Result = SubmitTooLong
		return s, nil
	}
	if typ < 0 || typ >= int32(typeCount) {
		s.Result = SubmitBadType
		return s, nil
	}
	id, err := m.create(Type(typ), player, content)
	if err != nil {
		s.Result = SubmitNoID
		return s, err
	}
	s.ID = id
	return s, nil
}

// create files a new pending petition; m.mu is held.
func (m *Manager) create(typ Type, petitioner Person, content string) (int32, error) {
	if m.ids == nil {
		return 0, fmt.Errorf("petition: no id allocator")
	}
	id, err := m.ids.NextID()
	if err != nil {
		return 0, fmt.Errorf("petition: allocate id: %w", err)
	}
	m.names[petitioner.ID] = petitioner.Name
	m.insert(&petition{Record: Record{
		ID:         id,
		Type:       typ,
		Petitioner: petitioner.ID,
		SubmitDate: m.now().UnixMilli(),
		Content:    content,
		Unread:     true,
		State:      Pending,
		Rate:       Fair,
	}})
	return id, nil
}

// CancelResult is how a petition cancel request ended.
type CancelResult int

// Cancel outcomes.
const (
	// CancelUnderProcess: the petitioner's petition is being answered;
	// only a responder may end it.
	CancelUnderProcess CancelResult = iota
	// CancelLeft: a responder ended the petition it answers (a game
	// master) or left its chat (anyone else).
	CancelLeft
	// CancelNotSubmitted: the player has no pending petition.
	CancelNotSubmitted
	// CancelDone: the player's pending petition was cancelled.
	CancelDone
)

// Cancel answers player's request to cancel: a petitioner's pending
// petition is cancelled, a game master answering a petition closes it and
// any other responder leaves its chat. gm is whether player plays as a game
// master. remaining is how many petitions player may still send, set on
// CancelDone.
func (m *Manager) Cancel(player Person, gm bool, pres Presence) (result CancelResult, remaining int, notices []Notice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.inProcess(player.ID); p != nil {
		if p.Petitioner == player.ID {
			return CancelUnderProcess, 0, nil
		}
		if gm {
			m.end(p, Closed, pres, &notices)
		} else {
			m.removeResponder(p, player, pres, &notices)
		}
		return CancelLeft, 0, notices
	}
	p := m.find(func(p *petition) bool { return p.State == Pending && p.Petitioner == player.ID })
	if p == nil {
		return CancelNotSubmitted, 0, nil
	}
	m.end(p, Cancelled, pres, &notices)
	return CancelDone, m.cfg.MaxPerPlayer - m.countOf(player.ID), notices
}

// Vote records the petitioner's rating of its closed petition. It reports
// false, and changes nothing, when player has no closed petition awaiting
// feedback or rate is none the client can name.
func (m *Manager) Vote(player int32, rate int32, feedback string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.find(func(p *petition) bool { return p.State == Closed && p.underFeedback && p.Petitioner == player })
	if p == nil || rate < 0 || rate >= int32(rateCount) {
		return false
	}
	p.underFeedback = false
	p.Rate = Rate(rate)
	p.Feedback = feedback
	return true
}

// Say says text in the chat of the petition player sent or answers. It
// reports false when player is in no petition being answered. gm is
// whether player plays as a game master.
func (m *Manager) Say(player Person, gm bool, text string, pres Presence) (notices []Notice, ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.inProcess(player.ID)
	if p == nil {
		return nil, false
	}
	channel := chat.PetitionPlayer
	if p.Petitioner != player.ID && gm {
		channel = chat.PetitionGM
	}
	msg := Message{ObjectID: player.ID, Channel: channel, Name: player.Name, Text: text}
	p.Messages = append(p.Messages, msg)
	m.toResponders(p, Notice{Kind: Say, Say: msg}, pres, &notices)
	m.toPetitioner(p, Notice{Kind: Say, Say: msg}, pres, &notices)
	return notices, true
}

// ActiveLog returns the chat of the first active petition, in id order,
// player sent or answers: what a player entering the world is shown.
func (m *Manager) ActiveLog(player int32) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.find(func(p *petition) bool { return active(p) && p.participant(player) })
	if p == nil {
		return nil
	}
	return slices.Clone(p.Messages)
}

// Log returns the chat of petition id, none when there is no such
// petition.
func (m *Manager) Log(id int32) []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.petitions[id]; ok {
		return slices.Clone(p.Messages)
	}
	return nil
}

// Join has gm answer petition id, leaving the petition it answered before.
// A pending petition becomes accepted with gm its responder; on an accepted
// one gm joins the responders. enforcing marks a petition gm opened itself
// for a player, which tells both sides differently. It reports false when
// there is no such petition or gm cannot join it.
func (m *Manager) Join(gm Person, id int32, enforcing bool, pres Presence) (bool, []Notice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var notices []Notice
	if p := m.inProcess(gm.ID); p != nil {
		m.abort(p, gm, pres, &notices)
	}
	p, ok := m.petitions[id]
	if !ok {
		return false, notices
	}
	return m.join(p, gm, enforcing, pres, &notices), notices
}

// Reject has gm reject petition id. It reports false when there is no such
// petition or it is rejected already.
func (m *Manager) Reject(gm Person, id int32, pres Presence) (bool, []Notice) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.petitions[id]
	if !ok || p.State == Rejected {
		return false, nil
	}
	var notices []Notice
	m.addResponder(p, gm)
	m.end(p, Rejected, pres, &notices)
	return true, notices
}

// Unfollow has player leave the petition it answers.
func (m *Manager) Unfollow(player Person, pres Presence) []Notice {
	m.mu.Lock()
	defer m.mu.Unlock()
	var notices []Notice
	if p := m.inProcess(player.ID); p != nil {
		m.abort(p, player, pres, &notices)
	}
	return notices
}

// Reset drops every petition and frees its id. It reports false, and drops
// nothing, while any petition is being answered.
func (m *Manager) Reset() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.petitions {
		if p.State == Accepted {
			return false
		}
	}
	if m.ids != nil {
		for _, id := range m.order {
			m.ids.ReleaseID(id)
		}
	}
	clear(m.petitions)
	m.order = nil
	return true
}

// AddResult is how adding a player to a petition chat ended. The nonzero
// values are the error numbers the game master is told.
type AddResult int32

// Add outcomes.
const (
	Added AddResult = iota
	// AddSelf: the game master named itself.
	AddSelf
	// AddNoPetition: the game master answers no petition.
	AddNoPetition
	// AddPetitioner: the target sent the petition.
	AddPetitioner
	// AddAlreadyResponder: the target answers the petition already.
	AddAlreadyResponder
)

// AddToChat adds target to the chat of the petition gm answers.
func (m *Manager) AddToChat(gm, target Person, pres Presence) (AddResult, []Notice) {
	if gm.ID == target.ID {
		return AddSelf, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.inProcess(gm.ID)
	switch {
	case p == nil:
		return AddNoPetition, nil
	case p.Petitioner == target.ID:
		return AddPetitioner, nil
	case p.responder(target.ID):
		return AddAlreadyResponder, nil
	}
	var notices []Notice
	m.addAdditional(p, gm, target, pres, &notices)
	return Added, notices
}

// ForceResult is how a game master's opening a petition for a player
// ended.
type ForceResult int

// Force outcomes.
const (
	Forced ForceResult = iota
	// ForceSelf: the game master named itself.
	ForceSelf
	// ForceAlreadySubmitted: the target has an active petition already.
	ForceAlreadySubmitted
	// ForceNoID: no object id was left to number the petition with.
	ForceNoID
)

// Force opens an empty petition of type Other for target, which gm then
// joins with Join. It returns the new petition's id.
func (m *Manager) Force(gm, target Person) (ForceResult, int32, error) {
	if gm.ID == target.ID {
		return ForceSelf, 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeOf(target.ID) {
		return ForceAlreadySubmitted, 0, nil
	}
	id, err := m.create(TypeOther, target, "")
	if err != nil {
		return ForceNoID, 0, err
	}
	return Forced, id, nil
}

// InProcess returns the id of the petition player sent or answers that is
// being answered.
func (m *Manager) InProcess(player int32) (int32, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.inProcess(player); p != nil {
		return p.ID, true
	}
	return 0, false
}

// Summary is one petition as the game masters' list shows it.
type Summary struct {
	ID             int32
	Type           Type
	State          State
	Unread         bool
	Petitioner     int32
	PetitionerName string
}

// List returns every petition in id order.
func (m *Manager) List() []Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Summary, 0, len(m.order))
	m.each(func(p *petition) bool {
		out = append(out, Summary{ID: p.ID, Type: p.Type, State: p.State, Unread: p.Unread, Petitioner: p.Petitioner, PetitionerName: m.names[p.Petitioner]})
		return true
	})
	return out
}

// Detail is one petition as a game master reads it.
type Detail struct {
	Summary
	SubmitDate int64
	Content    string
	Rate       Rate
	Feedback   string
	// Responders names each responder with a known name, each followed by
	// a space.
	Responders string
}

// Read returns petition id and marks it read.
func (m *Manager) Read(id int32) (Detail, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.petitions[id]
	if !ok {
		return Detail{}, false
	}
	p.Unread = false
	var responders []byte
	for _, r := range p.Responders {
		if name, ok := m.names[r]; ok {
			responders = append(append(responders, name...), ' ')
		}
	}
	return Detail{
		Summary:    Summary{ID: p.ID, Type: p.Type, State: p.State, Unread: p.Unread, Petitioner: p.Petitioner, PetitionerName: m.names[p.Petitioner]},
		SubmitDate: p.SubmitDate,
		Content:    p.Content,
		Rate:       p.Rate,
		Feedback:   p.Feedback,
		Responders: string(responders),
	}, true
}

// Records returns every petition in id order as it is stored. A petition
// accepted with nothing said in it yet is stored pending, with no
// responder.
func (m *Manager) Records() []Record {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Record, 0, len(m.order))
	m.each(func(p *petition) bool {
		r := p.Record
		r.Responders = slices.Clone(p.Responders)
		r.Messages = slices.Clone(p.Messages)
		if r.State == Accepted && len(r.Messages) == 0 {
			r.State = Pending
			r.Responders = nil
		}
		out = append(out, r)
		return true
	})
	return out
}

// addResponder makes person a responder of p unless it sent p or answers
// it already; m.mu is held.
func (m *Manager) addResponder(p *petition, person Person) bool {
	if person.ID == p.Petitioner || p.responder(person.ID) {
		return false
	}
	p.Responders = append(p.Responders, person.ID)
	m.names[person.ID] = person.Name
	return true
}

// addAdditional adds target to p's chat on actor's behalf; m.mu is held.
func (m *Manager) addAdditional(p *petition, actor, target Person, pres Presence, out *[]Notice) bool {
	if !m.addResponder(p, target) {
		*out = append(*out, Notice{To: actor.ID, Kind: FailedAdding, Name: target.Name})
		return false
	}
	if len(p.Messages) > 0 {
		showLog(p, target.ID, out)
	}
	m.toPetitioner(p, Notice{Kind: Participating, Name: target.Name}, pres, out)
	m.toResponders(p, Notice{Kind: Participating, Name: target.Name}, pres, out)
	return true
}

// removeResponder has person leave p's chat; m.mu is held.
func (m *Manager) removeResponder(p *petition, person Person, pres Presence, out *[]Notice) {
	m.toPetitioner(p, Notice{Kind: LeftChat, Name: person.Name}, pres, out)
	m.toResponders(p, Notice{Kind: LeftChat, Name: person.Name}, pres, out)
	if i := slices.Index(p.Responders, person.ID); i >= 0 {
		p.Responders = slices.Delete(p.Responders, i, i+1)
	}
}

// join has person answer p; m.mu is held.
func (m *Manager) join(p *petition, person Person, enforcing bool, pres Presence, out *[]Notice) bool {
	if p.State == Accepted {
		return m.addAdditional(p, person, person, pres, out)
	}
	if p.State != Pending || !m.addResponder(p, person) {
		return false
	}
	p.State = Accepted
	switch {
	case len(p.Messages) > 0:
		showLog(p, person.ID, out)
	case enforcing:
		m.toPetitioner(p, Notice{Kind: ConsultationReceived, Name: m.names[p.Petitioner]}, pres, out)
		m.toResponders(p, Notice{Kind: ReceivedCode, Name: person.Name, Number: p.ID}, pres, out)
	default:
		m.toPetitioner(p, Notice{Kind: ApplicationAccepted}, pres, out)
		m.toResponders(p, Notice{Kind: UnderWay, Name: m.names[p.Petitioner]}, pres, out)
	}
	return true
}

// abort has person stop answering p. When no other game master in the
// world answers it, or person was its only responder, p goes back to
// pending; otherwise person only leaves its chat. m.mu is held.
func (m *Manager) abort(p *petition, person Person, pres Presence, out *[]Notice) {
	lastGM := true
	for _, r := range p.Responders {
		if r == person.ID || !pres.Online(r) || !pres.GM(r) {
			continue
		}
		lastGM = false
		break
	}
	if len(p.Responders) == 1 || lastGM {
		m.end(p, Pending, pres, out)
		return
	}
	m.removeResponder(p, person, pres, out)
}

// end ends p's consultation in state, telling its responders and its
// petitioner; m.mu is held. A closed petition keeps its responders and
// awaits the petitioner's feedback; any other state drops them.
func (m *Manager) end(p *petition, state State, pres Presence, out *[]Notice) {
	name := m.names[p.Petitioner]
	p.State = state
	for _, r := range p.Responders {
		if !pres.Online(r) {
			continue
		}
		*out = append(*out, Notice{To: r, Kind: EndedWith, Name: name})
		if state == Cancelled {
			*out = append(*out, Notice{To: r, Kind: ReceiptCancelled, Number: p.ID})
		}
	}
	if pres.Online(p.Petitioner) {
		switch state {
		case Pending:
			*out = append(*out, Notice{To: p.Petitioner, Kind: LeftChat, Name: theGM})
		case Closed:
			p.underFeedback = true
			*out = append(*out, Notice{To: p.Petitioner, Kind: ProvideFeedback}, Notice{To: p.Petitioner, Kind: VoteWindow})
		}
	}
	if state != Closed {
		p.Responders = nil
	}
}

// toPetitioner sends n to p's petitioner when it is in the world; m.mu is
// held.
func (m *Manager) toPetitioner(p *petition, n Notice, pres Presence, out *[]Notice) {
	if !pres.Online(p.Petitioner) {
		return
	}
	n.To = p.Petitioner
	*out = append(*out, n)
}

// toResponders sends n to p's responders in the order they joined,
// stopping at the first one out of the world. With no responder left, p
// goes back to pending instead. m.mu is held.
func (m *Manager) toResponders(p *petition, n Notice, pres Presence, out *[]Notice) {
	if len(p.Responders) == 0 {
		m.end(p, Pending, pres, out)
		return
	}
	for _, r := range p.Responders {
		if !pres.Online(r) {
			return
		}
		n.To = r
		*out = append(*out, n)
	}
}

// showLog sends p's chat to player, one line per notice.
func showLog(p *petition, player int32, out *[]Notice) {
	for _, msg := range p.Messages {
		*out = append(*out, Notice{To: player, Kind: Say, Say: msg})
	}
}
