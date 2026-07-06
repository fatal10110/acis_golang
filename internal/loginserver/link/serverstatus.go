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

// serverStatusAttr identifies one ServerStatus attribute slot on the wire.
type serverStatusAttr int32

const (
	attrStatus     serverStatusAttr = 1
	attrClock      serverStatusAttr = 2
	attrBrackets   serverStatusAttr = 3
	attrAgeLimit   serverStatusAttr = 4
	attrTestServer serverStatusAttr = 5
	attrPvp        serverStatusAttr = 6
	attrMaxPlayers serverStatusAttr = 7
)

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
		switch serverStatusAttr(attr) {
		case attrStatus:
			st := ServerType(value)
			status.Status = &st
		case attrClock:
			on := value == serverStatusOn
			status.ShowClock = &on
		case attrBrackets:
			on := value == serverStatusOn
			status.ShowBrackets = &on
		case attrAgeLimit:
			status.AgeLimit = &value
		case attrTestServer:
			on := value == serverStatusOn
			status.TestServer = &on
		case attrPvp:
			on := value == serverStatusOn
			status.Pvp = &on
		case attrMaxPlayers:
			status.MaxPlayers = &value
		}
	}
	if r.Err() != nil {
		return ServerStatus{}, fmt.Errorf("link: ServerStatus: %w", r.Err())
	}
	return status, nil
}

// EncodeServerStatus builds the ServerStatus packet carrying status's
// non-nil attributes as (attribute, value) pairs.
func EncodeServerStatus(status ServerStatus) []byte {
	type attrValue struct {
		attr  serverStatusAttr
		value int32
	}
	var attrs []attrValue
	if status.Status != nil {
		attrs = append(attrs, attrValue{attrStatus, int32(*status.Status)})
	}
	if status.ShowClock != nil {
		attrs = append(attrs, attrValue{attrClock, onOffValue(*status.ShowClock)})
	}
	if status.ShowBrackets != nil {
		attrs = append(attrs, attrValue{attrBrackets, onOffValue(*status.ShowBrackets)})
	}
	if status.AgeLimit != nil {
		attrs = append(attrs, attrValue{attrAgeLimit, *status.AgeLimit})
	}
	if status.TestServer != nil {
		attrs = append(attrs, attrValue{attrTestServer, onOffValue(*status.TestServer)})
	}
	if status.Pvp != nil {
		attrs = append(attrs, attrValue{attrPvp, onOffValue(*status.Pvp)})
	}
	if status.MaxPlayers != nil {
		attrs = append(attrs, attrValue{attrMaxPlayers, *status.MaxPlayers})
	}

	w := newWriter(OpcodeServerStatus)
	w.WriteInt32(int32(len(attrs)))
	for _, a := range attrs {
		w.WriteInt32(int32(a.attr))
		w.WriteInt32(a.value)
	}
	return w.Bytes()
}

// onOffValue converts a boolean attribute to its wire representation.
func onOffValue(on bool) int32 {
	if on {
		return serverStatusOn
	}
	return 0
}
