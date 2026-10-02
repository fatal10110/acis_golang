// Package petition keeps the petitions players send the game masters: who
// sent each one, the game masters (and players they add) answering it, the
// chat said in it, and the feedback the petitioner leaves once it is
// closed.
package petition

import "github.com/fatal10110/acis_golang/internal/gameserver/handler/chat"

// Type is what a petition is about, as the client numbers it.
type Type int32

// Petition types.
const (
	TypeNone Type = iota
	TypeImmobility
	TypeRecoveryRelated
	TypeBugReport
	TypeQuestRelated
	TypeBadUser
	TypeSuggestions
	TypeGameTip
	TypeOperationRelated
	TypeOther
	typeCount
)

var typeNames = [typeCount]string{
	"NONE", "IMMOBILITY", "RECOVERY_RELATED", "BUG_REPORT", "QUEST_RELATED", "BAD_USER",
	"SUGGESTIONS", "GAME_TIP", "OPERATION_RELATED", "OTHER",
}

// String is the type's name as it is stored and shown to game masters.
func (t Type) String() string {
	if t < 0 || t >= typeCount {
		return ""
	}
	return typeNames[t]
}

// ParseType returns the type a stored name names.
func ParseType(name string) (Type, bool) { return parse[Type](typeNames[:], name) }

// State is where a petition stands.
type State int

// Petition states. A pending or accepted petition is active.
const (
	Pending State = iota
	Accepted
	Rejected
	Cancelled
	Closed
	stateCount
)

var stateNames = [stateCount]string{"PENDING", "ACCEPTED", "REJECTED", "CANCELLED", "CLOSED"}

// String is the state's name as it is stored and shown to game masters.
func (s State) String() string {
	if s < 0 || s >= stateCount {
		return ""
	}
	return stateNames[s]
}

// ParseState returns the state a stored name names.
func ParseState(name string) (State, bool) { return parse[State](stateNames[:], name) }

// Rate is the petitioner's rating of the answer it got.
type Rate int

// Rates, as the client numbers them.
const (
	VeryGood Rate = iota
	Good
	Fair
	Poor
	VeryPoor
	rateCount
)

var (
	rateNames = [rateCount]string{"VERY_GOOD", "GOOD", "FAIR", "POOR", "VERY_POOR"}
	rateDescs = [rateCount]string{"Very Good", "Good", "Fair", "Poor", "Very Poor"}
)

// String is the rate's name as it is stored.
func (r Rate) String() string {
	if r < 0 || r >= rateCount {
		return ""
	}
	return rateNames[r]
}

// Desc is the rate as a game master reads it.
func (r Rate) Desc() string {
	if r < 0 || r >= rateCount {
		return ""
	}
	return rateDescs[r]
}

// ParseRate returns the rate a stored name names.
func ParseRate(name string) (Rate, bool) { return parse[Rate](rateNames[:], name) }

func parse[T ~int | ~int32](names []string, name string) (T, bool) {
	for i, n := range names {
		if n == name {
			return T(i), true
		}
	}
	return 0, false
}

// Message is one line said in a petition's chat, kept for whoever joins it
// later.
type Message struct {
	ObjectID int32
	Channel  chat.Type
	Name     string
	Text     string
}

// Person is a player taking part in a petition.
type Person struct {
	ID   int32
	Name string
}

// Record is one petition as it is stored, its chat lines in the order they
// were said.
type Record struct {
	ID         int32
	Type       Type
	Petitioner int32
	SubmitDate int64 // epoch milliseconds
	Content    string
	Unread     bool
	State      State
	Rate       Rate
	Feedback   string
	Responders []int32
	Messages   []Message
}

// Config is the petition settings of players.properties.
type Config struct {
	// Allowed lets players petition at all (PetitioningAllowed).
	Allowed bool
	// MaxPerPlayer is how many petitions, cancelled ones aside, one player
	// may send (MaxPetitionsPerPlayer).
	MaxPerPlayer int
	// MaxPending is how many active petitions the server holds at once
	// (MaxPetitionsPending).
	MaxPending int
}

// DefaultConfig is the shipped petition settings.
func DefaultConfig() Config {
	return Config{Allowed: true, MaxPerPlayer: 5, MaxPending: 25}
}

// People returns every petitioner and responder records name, each once.
func People(records []Record) []int32 {
	seen := make(map[int32]struct{})
	var out []int32
	add := func(id int32) {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	for _, r := range records {
		add(r.Petitioner)
		for _, id := range r.Responders {
			add(id)
		}
	}
	return out
}
