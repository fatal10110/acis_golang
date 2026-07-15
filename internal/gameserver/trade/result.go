package trade

// RequestStatus describes the outcome of creating a direct-trade request.
type RequestStatus uint8

const (
	// RequestStarted means the target now has a pending request.
	RequestStarted RequestStatus = iota
	// RequestRequesterBusy means the requester already has a pending or active trade.
	RequestRequesterBusy
	// RequestTargetBusy means the target already has a pending or active trade.
	RequestTargetBusy
)

// RequestResult is returned after a direct-trade request attempt.
type RequestResult struct {
	Status RequestStatus
}

// AnswerStatus describes the outcome of answering a direct-trade request.
type AnswerStatus uint8

const (
	// AnswerMissing means no live request was found for the target.
	AnswerMissing AnswerStatus = iota
	// AnswerDenied means the request was rejected or expired.
	AnswerDenied
	// AnswerAccepted means an active trade session was created.
	AnswerAccepted
)

// AnswerResult is returned after a direct-trade answer.
type AnswerResult struct {
	Status      AnswerStatus
	RequesterID int32
	TargetID    int32
}

// AddStatus describes the outcome of adding an item to an active offer.
type AddStatus uint8

const (
	// AddAccepted means the item was added to the player's offer.
	AddAccepted AddStatus = iota
	// AddNoSession means the player has no mutable active session.
	AddNoSession
	// AddSelfConfirmed means the player already confirmed the trade.
	AddSelfConfirmed
	// AddPartnerConfirmed means the partner already confirmed the trade.
	AddPartnerConfirmed
	// AddInvalidItem means the requested item cannot be offered.
	AddInvalidItem
)

// AddResult is returned after adding an item to a direct-trade offer.
type AddResult struct {
	Status         AddStatus
	PartnerID      int32
	Item           ItemSnapshot
	AddedCount     int
	AvailableCount int
	Entries        []ItemUpdateEntry
}

// DoneStatus describes the outcome of confirming or completing a trade.
type DoneStatus uint8

const (
	// DoneNoSession means the player has no active session.
	DoneNoSession DoneStatus = iota
	// DoneAlreadyConfirmed means the player had already confirmed this session.
	DoneAlreadyConfirmed
	// DoneConfirmed means one side has confirmed and the other side must still answer.
	DoneConfirmed
	// DoneReady means both sides confirmed and the returned session is ready to commit.
	DoneReady
)

// DoneResult is returned after a player confirms a trade.
type DoneResult struct {
	Status    DoneStatus
	PartnerID int32
	Session   Session
}

// CancelStatus describes whether an active session was canceled.
type CancelStatus uint8

const (
	// CancelMissing means no cancelable active session was found.
	CancelMissing CancelStatus = iota
	// CancelDone means a session was canceled.
	CancelDone
)

// CancelResult is returned after canceling an active session.
type CancelResult struct {
	Status  CancelStatus
	Session Session
}

// ReceiverStatus describes whether a receiver can accept an offer.
type ReceiverStatus uint8

const (
	// ReceiverOK means the receiver can accept the offer.
	ReceiverOK ReceiverStatus = iota
	// ReceiverWeightExceeded means the offer would exceed the receiver's weight limit.
	ReceiverWeightExceeded
	// ReceiverSlotsFull means the offer would exceed the receiver's slot limit.
	ReceiverSlotsFull
)
