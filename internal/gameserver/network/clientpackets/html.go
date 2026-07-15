package clientpackets

import "fmt"

// RequestLinkHTML asks the server to send a cached datapack HTML page.
type RequestLinkHTML struct {
	Link string
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
