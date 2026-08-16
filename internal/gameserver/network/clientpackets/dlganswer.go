package clientpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

const dlgAnswerSize = 12

// DlgAnswer is the client's response to a ConfirmDlg, matching
// DlgAnswer.java: a message id identifying which dialog is being answered,
// the accept/decline choice (1 accepts), and the requester's object id the
// server echoed into the dialog.
type DlgAnswer struct {
	MessageID   int32
	Answer      int32
	RequesterID int32
}

// DecodeDlgAnswer parses a raw DlgAnswer payload (opcode byte included).
func DecodeDlgAnswer(payload []byte) (DlgAnswer, error) {
	r := newReader(payload)
	if r.Remaining() < dlgAnswerSize {
		return DlgAnswer{}, fmt.Errorf("clientpackets: DlgAnswer: need %d bytes, got %d: %w", dlgAnswerSize, r.Remaining(), wire.ErrShortPacket)
	}
	return DlgAnswer{
		MessageID:   r.ReadInt32(),
		Answer:      r.ReadInt32(),
		RequesterID: r.ReadInt32(),
	}, nil
}
