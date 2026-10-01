package clientpackets

import "fmt"

// Friend and block list request opcodes.
const (
	OpcodeRequestFriendInvite       = 0x5e
	OpcodeRequestAnswerFriendInvite = 0x5f
	OpcodeRequestFriendList         = 0x60
	OpcodeRequestFriendDel          = 0x61
	OpcodeRequestBlock              = 0xa0
	OpcodeRequestSendL2FriendSay    = 0xcc
)

// RequestFriendInvite invites the named player onto the sender's friend
// list.
type RequestFriendInvite struct {
	Name string
}

// DecodeRequestFriendInvite parses a raw RequestFriendInvite payload (opcode
// byte included).
func DecodeRequestFriendInvite(payload []byte) (RequestFriendInvite, error) {
	r := newReader(payload)
	req := RequestFriendInvite{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestFriendInvite{}, fmt.Errorf("clientpackets: RequestFriendInvite: %w", err)
	}
	return req, nil
}

// RequestAnswerFriendInvite answers the friend invitation the sender holds;
// Response 1 accepts.
type RequestAnswerFriendInvite struct {
	Response int32
}

// DecodeRequestAnswerFriendInvite parses a raw RequestAnswerFriendInvite
// payload (opcode byte included).
func DecodeRequestAnswerFriendInvite(payload []byte) (RequestAnswerFriendInvite, error) {
	r := newReader(payload)
	req := RequestAnswerFriendInvite{Response: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestAnswerFriendInvite{}, fmt.Errorf("clientpackets: RequestAnswerFriendInvite: %w", err)
	}
	return req, nil
}

// RequestFriendDel removes the named character from the sender's friend
// list.
type RequestFriendDel struct {
	Name string
}

// DecodeRequestFriendDel parses a raw RequestFriendDel payload (opcode byte
// included).
func DecodeRequestFriendDel(payload []byte) (RequestFriendDel, error) {
	r := newReader(payload)
	req := RequestFriendDel{Name: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestFriendDel{}, fmt.Errorf("clientpackets: RequestFriendDel: %w", err)
	}
	return req, nil
}

// RequestBlock types.
const (
	BlockAdd        int32 = 0
	BlockRemove     int32 = 1
	BlockList       int32 = 2
	BlockAll        int32 = 3
	BlockAllRelease int32 = 4
)

// RequestBlock is one /block, /unblock, /blocklist, /allblock or
// /allunblock command. Name is read only for a block or an unblock.
type RequestBlock struct {
	Type int32
	Name string
}

// DecodeRequestBlock parses a raw RequestBlock payload (opcode byte
// included).
func DecodeRequestBlock(payload []byte) (RequestBlock, error) {
	r := newReader(payload)
	req := RequestBlock{Type: r.ReadInt32()}
	if req.Type == BlockAdd || req.Type == BlockRemove {
		req.Name = r.ReadString()
	}
	if err := r.Err(); err != nil {
		return RequestBlock{}, fmt.Errorf("clientpackets: RequestBlock: %w", err)
	}
	return req, nil
}

// RequestSendL2FriendSay sends a private message to a friend.
type RequestSendL2FriendSay struct {
	Message   string
	Recipient string
}

// DecodeRequestSendL2FriendSay parses a raw RequestSendL2FriendSay payload
// (opcode byte included).
func DecodeRequestSendL2FriendSay(payload []byte) (RequestSendL2FriendSay, error) {
	r := newReader(payload)
	req := RequestSendL2FriendSay{Message: r.ReadString()}
	req.Recipient = r.ReadString()
	if err := r.Err(); err != nil {
		return RequestSendL2FriendSay{}, fmt.Errorf("clientpackets: RequestSendL2FriendSay: %w", err)
	}
	return req, nil
}
