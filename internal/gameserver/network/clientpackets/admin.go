package clientpackets

import "fmt"

const (
	// OpcodeSendBypassBuildCmd carries an administrator command typed in
	// chat as "//command".
	OpcodeSendBypassBuildCmd = 0x5b
	// OpcodeRequestGmList asks for the online game masters (/gmlist).
	OpcodeRequestGmList = 0x81
)

// SendBypassBuildCmd is an administrator command typed in chat, without its
// "//" and as the client sent it.
type SendBypassBuildCmd struct {
	Command string
}

// DecodeSendBypassBuildCmd parses a raw SendBypassBuildCmd payload (opcode
// byte included).
func DecodeSendBypassBuildCmd(payload []byte) (SendBypassBuildCmd, error) {
	r := newReader(payload)
	req := SendBypassBuildCmd{Command: r.ReadString()}
	if err := r.Err(); err != nil {
		return SendBypassBuildCmd{}, fmt.Errorf("clientpackets: SendBypassBuildCmd: %w", err)
	}
	return req, nil
}
