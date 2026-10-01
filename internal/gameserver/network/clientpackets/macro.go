package clientpackets

import (
	"fmt"
)

// Macro window and recommendation opcodes.
const (
	OpcodeRequestEvaluate    = 0xb9
	OpcodeRequestMakeMacro   = 0xc1
	OpcodeRequestDeleteMacro = 0xc2
	maxMakeMacroCommandsRead = 12
)

// MacroCommand is one decoded macro line. The line number the client
// sends ahead of it is read and dropped.
type MacroCommand struct {
	Type int32
	D1   int32
	D2   int32
	Text string
}

// RequestMakeMacro creates a macro (ID 0) or replaces the macro with ID.
type RequestMakeMacro struct {
	ID          int32
	Name        string
	Description string
	Acronym     string
	Icon        int32
	// Commands holds at most 12 lines: a larger count is read as 12 and
	// the lines past it are ignored.
	Commands []MacroCommand
}

// DecodeRequestMakeMacro parses a raw RequestMakeMacro payload (opcode byte
// included).
func DecodeRequestMakeMacro(payload []byte) (RequestMakeMacro, error) {
	r := newReader(payload)
	req := RequestMakeMacro{
		ID:          r.ReadInt32(),
		Name:        r.ReadString(),
		Description: r.ReadString(),
		Acronym:     r.ReadString(),
		Icon:        int32(r.ReadUint8()),
	}
	count := min(int(r.ReadUint8()), maxMakeMacroCommandsRead)
	for range count {
		if r.Err() != nil {
			break
		}
		r.ReadUint8() // line number
		req.Commands = append(req.Commands, MacroCommand{
			Type: int32(r.ReadUint8()),
			D1:   r.ReadInt32(),
			D2:   int32(r.ReadUint8()),
			Text: r.ReadString(),
		})
	}
	if err := r.Err(); err != nil {
		return RequestMakeMacro{}, fmt.Errorf("clientpackets: RequestMakeMacro: %w", err)
	}
	return req, nil
}

// RequestDeleteMacro deletes the macro with ID.
type RequestDeleteMacro struct {
	ID int32
}

// DecodeRequestDeleteMacro parses a raw RequestDeleteMacro payload (opcode
// byte included).
func DecodeRequestDeleteMacro(payload []byte) (RequestDeleteMacro, error) {
	r := newReader(payload)
	req := RequestDeleteMacro{ID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestDeleteMacro{}, fmt.Errorf("clientpackets: RequestDeleteMacro: %w", err)
	}
	return req, nil
}

// RequestEvaluate recommends the player with TargetID.
type RequestEvaluate struct {
	TargetID int32
}

// DecodeRequestEvaluate parses a raw RequestEvaluate payload (opcode byte
// included).
func DecodeRequestEvaluate(payload []byte) (RequestEvaluate, error) {
	r := newReader(payload)
	req := RequestEvaluate{TargetID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestEvaluate{}, fmt.Errorf("clientpackets: RequestEvaluate: %w", err)
	}
	return req, nil
}
