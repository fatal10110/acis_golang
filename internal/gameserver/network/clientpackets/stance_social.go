package clientpackets

import "fmt"

// RequestChangeMoveType asks the server to toggle walking/running.
type RequestChangeMoveType struct {
	Run bool
}

// DecodeRequestChangeMoveType parses a raw RequestChangeMoveType payload
// (opcode byte included).
func DecodeRequestChangeMoveType(payload []byte) (RequestChangeMoveType, error) {
	v, err := decodeSingleInt32(payload, "RequestChangeMoveType")
	return RequestChangeMoveType{Run: v == 1}, err
}

// RequestChangeWaitType asks the server to toggle sitting/standing.
type RequestChangeWaitType struct {
	Stand bool
}

// DecodeRequestChangeWaitType parses a raw RequestChangeWaitType payload
// (opcode byte included).
func DecodeRequestChangeWaitType(payload []byte) (RequestChangeWaitType, error) {
	v, err := decodeSingleInt32(payload, "RequestChangeWaitType")
	return RequestChangeWaitType{Stand: v == 1}, err
}

// RequestSocialAction asks the server to broadcast a social animation.
type RequestSocialAction struct {
	ActionID int32
}

// DecodeRequestSocialAction parses a raw RequestSocialAction payload (opcode
// byte included).
func DecodeRequestSocialAction(payload []byte) (RequestSocialAction, error) {
	v, err := decodeSingleInt32(payload, "RequestSocialAction")
	return RequestSocialAction{ActionID: v}, err
}

func decodeSingleInt32(payload []byte, name string) (int32, error) {
	r := newReader(payload)
	if r.Remaining() < 4 {
		return 0, fmt.Errorf("clientpackets: %s: need 4 bytes, got %d", name, r.Remaining())
	}
	v := r.ReadInt32()
	if err := r.Err(); err != nil {
		return 0, fmt.Errorf("clientpackets: %s: %w", name, err)
	}
	return v, nil
}
