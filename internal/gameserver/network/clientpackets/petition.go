package clientpackets

import "fmt"

const (
	// OpcodeRequestPetition sends the game masters a petition.
	OpcodeRequestPetition = 0x7f
	// OpcodeRequestPetitionCancel cancels the player's petition, or ends
	// or leaves the one it answers.
	OpcodeRequestPetitionCancel = 0x80
	// OpcodePetitionVote rates the answer to a closed petition.
	OpcodePetitionVote = 0xc8
)

// RequestPetition is a petition: its text and type.
type RequestPetition struct {
	Content string
	Type    int32
}

// DecodeRequestPetition parses a raw RequestPetition payload (opcode byte
// included).
func DecodeRequestPetition(payload []byte) (RequestPetition, error) {
	r := newReader(payload)
	req := RequestPetition{Content: r.ReadString(), Type: r.ReadInt32()}
	if err := r.Err(); err != nil {
		return RequestPetition{}, fmt.Errorf("clientpackets: RequestPetition: %w", err)
	}
	return req, nil
}

// PetitionVote is the petitioner's rating of a closed petition and its
// feedback text.
type PetitionVote struct {
	Rate     int32
	Feedback string
}

// DecodePetitionVote parses a raw PetitionVote payload (opcode byte
// included). Its first field, always 1, is skipped.
func DecodePetitionVote(payload []byte) (PetitionVote, error) {
	r := newReader(payload)
	r.ReadInt32()
	req := PetitionVote{Rate: r.ReadInt32(), Feedback: r.ReadString()}
	if err := r.Err(); err != nil {
		return PetitionVote{}, fmt.Errorf("clientpackets: PetitionVote: %w", err)
	}
	return req, nil
}
