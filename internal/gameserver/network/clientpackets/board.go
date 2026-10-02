package clientpackets

import "fmt"

// Community board request opcodes.
const (
	OpcodeRequestBBSWrite  = 0x22
	OpcodeRequestShowBoard = 0x57
)

// DecodeRequestShowBoard parses a raw RequestShowBoard payload (opcode byte
// included). Its one int field is read and ignored: the board always opens
// on the configured default page.
func DecodeRequestShowBoard(payload []byte) (struct{}, error) {
	r := newReader(payload)
	r.ReadInt32()
	if err := r.Err(); err != nil {
		return struct{}{}, fmt.Errorf("clientpackets: RequestShowBoard: %w", err)
	}
	return struct{}{}, nil
}

// RequestBBSWrite submits a board form: the page's URL and the five
// arguments its write button names.
type RequestBBSWrite struct {
	URL  string
	Args [5]string
}

// DecodeRequestBBSWrite parses a raw RequestBBSwrite payload (opcode byte
// included).
func DecodeRequestBBSWrite(payload []byte) (RequestBBSWrite, error) {
	r := newReader(payload)
	req := RequestBBSWrite{URL: r.ReadString()}
	for i := range req.Args {
		req.Args[i] = r.ReadString()
	}
	if err := r.Err(); err != nil {
		return RequestBBSWrite{}, fmt.Errorf("clientpackets: RequestBBSwrite: %w", err)
	}
	return req, nil
}
