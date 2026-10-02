package petition

// NoticeKind is what a Notice tells its recipient.
type NoticeKind int

// Notice kinds. Each names the system message the client is sent, with the
// parameters it takes, except Say and VoteWindow.
const (
	// Say is a chat line of the petition: Say.
	Say NoticeKind = iota
	// VoteWindow opens the petitioner's feedback window.
	VoteWindow
	// Participating is "Name participates in the petition chat": Name.
	Participating
	// LeftChat is "Name left the petition chat": Name.
	LeftChat
	// FailedAdding is "Failed to add Name to the petition chat": Name.
	FailedAdding
	// ConsultationReceived is "Name received a consultation request": Name.
	ConsultationReceived
	// ReceivedCode is "Petition application from Name received, code
	// Number": Name, Number.
	ReceivedCode
	// ApplicationAccepted is "Petition application accepted".
	ApplicationAccepted
	// UnderWay is "Petition consultation with Name under way": Name.
	UnderWay
	// EndedWith is "Ending petition consultation with Name": Name.
	EndedWith
	// ReceiptCancelled is "Receipt No. Number petition cancelled": Number.
	ReceiptCancelled
	// ProvideFeedback is "This ends the GM petition consultation, please
	// take a moment to provide feedback".
	ProvideFeedback
)

// Notice is one packet a petition change sends one player.
type Notice struct {
	To     int32
	Kind   NoticeKind
	Name   string
	Number int32
	Say    Message
}

// Presence answers who is in the world while a petition change decides
// whom it reaches.
type Presence interface {
	// Online reports whether the player is in the world.
	Online(objectID int32) bool
	// GM reports whether the player is in the world playing as a game
	// master.
	GM(objectID int32) bool
}

// theGM is the name the petitioner is told left its chat when the last
// game master answering it leaves.
const theGM = "The GM"
