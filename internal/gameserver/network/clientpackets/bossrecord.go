package clientpackets

import "fmt"

// OpcodeRequestGetBossRecord is the extended sub-opcode of
// RequestGetBossRecord.
const OpcodeRequestGetBossRecord uint16 = 0x0018

// RequestGetBossRecord asks for the requester's raid point record. BossID
// is read but plays no part in the answer.
type RequestGetBossRecord struct {
	BossID int32
}

// DecodeRequestGetBossRecord parses a raw RequestGetBossRecord payload
// (extended opcode included).
func DecodeRequestGetBossRecord(payload []byte) (RequestGetBossRecord, error) {
	r, err := newExtendedReader(payload, "RequestGetBossRecord", OpcodeRequestGetBossRecord, 2)
	if err != nil {
		return RequestGetBossRecord{}, err
	}
	req := RequestGetBossRecord{BossID: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestGetBossRecord{}, fmt.Errorf("clientpackets: RequestGetBossRecord: %w", err)
	}
	return req, nil
}
