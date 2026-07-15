package clientpackets

import "fmt"

// RequestLinkHTML asks the server to send a cached datapack HTML page.
type RequestLinkHTML struct {
	Link string
}

// RequestBypassToServer asks the server to route a client bypass command.
type RequestBypassToServer struct {
	Command string
}

// DecodeRequestLinkHTML parses a raw RequestLinkHtml payload (opcode byte
// included).
func DecodeRequestLinkHTML(payload []byte) (RequestLinkHTML, error) {
	r := newReader(payload)
	req := RequestLinkHTML{Link: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestLinkHTML{}, fmt.Errorf("clientpackets: RequestLinkHtml: %w", err)
	}
	return req, nil
}

// DecodeRequestBypassToServer parses a raw RequestBypassToServer payload
// (opcode byte included).
func DecodeRequestBypassToServer(payload []byte) (RequestBypassToServer, error) {
	r := newReader(payload)
	req := RequestBypassToServer{Command: r.ReadString()}
	if err := r.Err(); err != nil {
		return RequestBypassToServer{}, fmt.Errorf("clientpackets: RequestBypassToServer: %w", err)
	}
	return req, nil
}
