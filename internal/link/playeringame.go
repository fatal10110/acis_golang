package link

import (
	"fmt"
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

// OpcodePlayerInGame is the wire opcode for PlayerInGame, reporting accounts
// that just entered the game on this server.
const OpcodePlayerInGame = 0x02

// DecodePlayerInGame parses a raw PlayerInGame payload (opcode byte
// included) into the list of accounts that entered the game.
func DecodePlayerInGame(payload []byte) ([]string, error) {
	r := newReader(payload)
	count := int(r.ReadUint16())
	// Every account is at least its 2-byte terminator, so the bytes present
	// bound both the capacity and the loop: the count alone would let a
	// short packet reserve, then append, up to 65535 accounts.
	accounts := make([]string, 0, min(count, r.Remaining()/2))
	for i := 0; i < count && r.Err() == nil; i++ {
		accounts = append(accounts, r.ReadString())
	}
	if r.Err() != nil {
		return nil, fmt.Errorf("link: PlayerInGame: %w", r.Err())
	}
	return accounts, nil
}

// EncodePlayerInGame builds the PlayerInGame packet reporting accounts that
// just entered the game on this server.
func EncodePlayerInGame(accounts []string) ([]byte, error) {
	count, err := wire.Uint16Count(len(accounts))
	if err != nil {
		return nil, err
	}
	w := newWriter(OpcodePlayerInGame)
	w.WriteUint16(count)
	for _, account := range accounts {
		w.WriteString(account)
	}
	return w.Bytes(), nil
}

// playerInGameHeaderSize is a PlayerInGame payload's opcode and account
// count.
const playerInGameHeaderSize = 3

// EncodePlayerInGameBatches splits accounts, in order, into as few
// PlayerInGame payloads as fit maxPayload bytes each, so a roster too large
// for one link frame still reaches the login server whole. An account too
// long to fit a payload on its own is an error, reported before any payload
// is built.
func EncodePlayerInGameBatches(accounts []string, maxPayload int) ([][]byte, error) {
	var batches [][]byte
	for start := 0; start < len(accounts); {
		size, end := playerInGameHeaderSize, start
		for end < len(accounts) && end-start < math.MaxUint16 {
			n := encodedStringSize(accounts[end])
			if size+n > maxPayload {
				break
			}
			size += n
			end++
		}
		if end == start {
			return nil, fmt.Errorf("link: PlayerInGame: account of %d bytes does not fit a %d-byte payload", encodedStringSize(accounts[start]), maxPayload)
		}
		payload, err := EncodePlayerInGame(accounts[start:end])
		if err != nil {
			return nil, err
		}
		batches = append(batches, payload)
		start = end
	}
	return batches, nil
}

// encodedStringSize is how many bytes wire.Writer.WriteString writes for s:
// its UTF-16 code units and the terminator, two bytes each.
func encodedStringSize(s string) int {
	n := 2
	for _, r := range s {
		if r < 0x10000 {
			n += 2
		} else {
			n += 4
		}
	}
	return n
}
