package link

import "fmt"

// OpcodeServerStatus is the wire opcode for ServerStatus, a game server
// pushing updated status attributes about itself.
const OpcodeServerStatus = 0x06

// ServerType is a game server's advertised status.
type ServerType int32

// Server status values, in wire order.
const (
	ServerTypeAuto ServerType = iota
	ServerTypeGood
	ServerTypeNormal
	ServerTypeFull
	ServerTypeDown
	ServerTypeGMOnly
)

func (t ServerType) String() string {
	switch t {
	case ServerTypeAuto:
		return "Auto"
	case ServerTypeGood:
		return "Good"
	case ServerTypeNormal:
		return "Normal"
	case ServerTypeFull:
		return "Full"
	case ServerTypeDown:
		return "Down"
	case ServerTypeGMOnly:
		return "Gm Only"
	default:
		return fmt.Sprintf("ServerType(%d)", int32(t))
	}
}

// serverStatusOn is the wire value meaning an on/off attribute is set.
const serverStatusOn = 1

// ServerStatus carries the subset of status attributes a game server chose
// to update; each field is nil when this update left that attribute
// unchanged.
type ServerStatus struct {
	Status       *ServerType
	ShowClock    *bool
	ShowBrackets *bool
	AgeLimit     *int32
	TestServer   *bool
	Pvp          *bool
	MaxPlayers   *int32
}

// DecodeServerStatus parses a raw ServerStatus payload (opcode byte
// included): a count-prefixed list of (attribute, value) pairs.
func DecodeServerStatus(payload []byte) (ServerStatus, error) {
	r := newReader(payload)
	count := int(r.ReadInt32())

	var status ServerStatus
	for i := 0; i < count && r.Err() == nil; i++ {
		attr := r.ReadInt32()
		value := r.ReadInt32()
		if r.Err() != nil {
			break
		}
		switch attr {
		case 1: // status
			st := ServerType(value)
			status.Status = &st
		case 2: // clock
			on := value == serverStatusOn
			status.ShowClock = &on
		case 3: // brackets
			on := value == serverStatusOn
			status.ShowBrackets = &on
		case 4: // age limit
			status.AgeLimit = &value
		case 5: // test server
			on := value == serverStatusOn
			status.TestServer = &on
		case 6: // pvp
			on := value == serverStatusOn
			status.Pvp = &on
		case 7: // max players
			status.MaxPlayers = &value
		}
	}
	if r.Err() != nil {
		return ServerStatus{}, fmt.Errorf("link: ServerStatus: %w", r.Err())
	}
	return status, nil
}
