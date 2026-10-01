package clientpackets

import (
	"fmt"
)

// Party-matching client packet opcodes.
const (
	OpcodeRequestListPartyWaiting = 0x6f
	OpcodeRequestManagePartyRoom  = 0x70
	OpcodeRequestJoinPartyRoom    = 0x71

	OpcodeRequestOustFromPartyRoom            uint16 = 0x0001
	OpcodeRequestDismissPartyRoom             uint16 = 0x0002
	OpcodeRequestWithdrawPartyRoom            uint16 = 0x0003
	OpcodeRequestAskJoinPartyRoom             uint16 = 0x0014
	OpcodeAnswerJoinPartyRoom                 uint16 = 0x0015
	OpcodeRequestListPartyMatchingWaitingRoom uint16 = 0x0016
	OpcodeRequestExitPartyMatchingWaitingRoom uint16 = 0x0017
)

// RequestListPartyWaiting opens the party-matching window: the rooms at
// Location (or near the player, or anywhere) whose level range LevelMode
// filters.
type RequestListPartyWaiting struct {
	Auto      int32
	Location  int32
	LevelMode int32
}

// DecodeRequestListPartyWaiting parses a raw RequestListPartyWaiting
// payload (opcode byte included).
func DecodeRequestListPartyWaiting(payload []byte) (RequestListPartyWaiting, error) {
	r := newReader(payload)
	req := RequestListPartyWaiting{Auto: r.ReadInt32(), Location: r.ReadInt32(), LevelMode: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestListPartyWaiting{}, fmt.Errorf("clientpackets: RequestListPartyWaiting: %w", err)
	}
	return req, nil
}

// RequestManagePartyRoom opens a room (RoomID 0) or revises the room
// RoomID.
type RequestManagePartyRoom struct {
	RoomID     int32
	MaxMembers int32
	MinLevel   int32
	MaxLevel   int32
	Loot       int32
	Title      string
}

// DecodeRequestManagePartyRoom parses a raw RequestManagePartyRoom payload
// (opcode byte included).
func DecodeRequestManagePartyRoom(payload []byte) (RequestManagePartyRoom, error) {
	r := newReader(payload)
	req := RequestManagePartyRoom{
		RoomID:     r.ReadInt32(),
		MaxMembers: r.ReadInt32(),
		MinLevel:   r.ReadInt32(),
		MaxLevel:   r.ReadInt32(),
		Loot:       r.ReadInt32(),
		Title:      r.ReadString(),
	}
	if err := r.Err(); err != nil {
		return RequestManagePartyRoom{}, fmt.Errorf("clientpackets: RequestManagePartyRoom: %w", err)
	}
	return req, nil
}

// RequestJoinPartyRoom enters the room RoomID, or with RoomID 0 the first
// room the window's Location and LevelMode would list.
type RequestJoinPartyRoom struct {
	RoomID    int32
	Location  int32
	LevelMode int32
}

// DecodeRequestJoinPartyRoom parses a raw RequestJoinPartyRoom payload
// (opcode byte included).
func DecodeRequestJoinPartyRoom(payload []byte) (RequestJoinPartyRoom, error) {
	r := newReader(payload)
	req := RequestJoinPartyRoom{RoomID: r.ReadInt32(), Location: r.ReadInt32(), LevelMode: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestJoinPartyRoom{}, fmt.Errorf("clientpackets: RequestJoinPartyRoom: %w", err)
	}
	return req, nil
}

// ObjectTarget names the player a request acts on by object id.
type ObjectTarget struct {
	ObjectID int32
}

// DecodeRequestOustFromPartyRoom parses a raw RequestOustFromPartyRoom
// payload (opcode byte and sub-opcode included).
func DecodeRequestOustFromPartyRoom(payload []byte) (ObjectTarget, error) {
	r, err := newExtendedReader(payload, "RequestOustFromPartyRoom", OpcodeRequestOustFromPartyRoom, 2+4)
	if err != nil {
		return ObjectTarget{}, err
	}
	return ObjectTarget{ObjectID: r.ReadInt32()}, nil
}

// PartyRoomRef names a room a request acts on.
type PartyRoomRef struct {
	RoomID int32
}

// DecodeRequestDismissPartyRoom parses a raw RequestDismissPartyRoom
// payload (opcode byte and sub-opcode included): the room id, then an
// unused int.
func DecodeRequestDismissPartyRoom(payload []byte) (PartyRoomRef, error) {
	return decodePartyRoomRef(payload, "RequestDismissPartyRoom", OpcodeRequestDismissPartyRoom)
}

// DecodeRequestWithdrawPartyRoom parses a raw RequestWithdrawPartyRoom
// payload (opcode byte and sub-opcode included): the room id, then an
// unused int.
func DecodeRequestWithdrawPartyRoom(payload []byte) (PartyRoomRef, error) {
	return decodePartyRoomRef(payload, "RequestWithdrawPartyRoom", OpcodeRequestWithdrawPartyRoom)
}

func decodePartyRoomRef(payload []byte, name string, opcode uint16) (PartyRoomRef, error) {
	r, err := newExtendedReader(payload, name, opcode, 2+4+4)
	if err != nil {
		return PartyRoomRef{}, err
	}
	return PartyRoomRef{RoomID: r.ReadInt32()}, nil
}

// DecodeRequestAskJoinPartyRoom parses a raw RequestAskJoinPartyRoom
// payload (opcode byte and sub-opcode included).
func DecodeRequestAskJoinPartyRoom(payload []byte) (TargetName, error) {
	return decodeExtendedName(payload, "RequestAskJoinPartyRoom", OpcodeRequestAskJoinPartyRoom)
}

// DecodeAnswerJoinPartyRoom parses a raw AnswerJoinPartyRoom payload
// (opcode byte and sub-opcode included); 1 accepts.
func DecodeAnswerJoinPartyRoom(payload []byte) (Response, error) {
	r, err := newExtendedReader(payload, "AnswerJoinPartyRoom", OpcodeAnswerJoinPartyRoom, 2+4)
	if err != nil {
		return Response{}, err
	}
	return Response{Response: r.ReadInt32()}, nil
}

// RequestListPartyMatchingWaitingRoom asks for the waiting players whose
// level lies in [MinLevel, MaxLevel]. Page is not used: the whole list
// comes back.
type RequestListPartyMatchingWaitingRoom struct {
	Page     int32
	MinLevel int32
	MaxLevel int32
	Mode     int32
}

// DecodeRequestListPartyMatchingWaitingRoom parses a raw
// RequestListPartyMatchingWaitingRoom payload (opcode byte and sub-opcode
// included).
func DecodeRequestListPartyMatchingWaitingRoom(payload []byte) (RequestListPartyMatchingWaitingRoom, error) {
	r, err := newExtendedReader(payload, "RequestListPartyMatchingWaitingRoom", OpcodeRequestListPartyMatchingWaitingRoom, 2+16)
	if err != nil {
		return RequestListPartyMatchingWaitingRoom{}, err
	}
	return RequestListPartyMatchingWaitingRoom{Page: r.ReadInt32(), MinLevel: r.ReadInt32(), MaxLevel: r.ReadInt32(), Mode: r.ReadInt32()}, nil
}
