package serverpackets

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
)

func TestFrameExSendManorList(t *testing.T) {
	got := framePayload(t, FrameExSendManorList())

	var want []byte
	want = append(want, OpcodeExtended)
	want = binary.LittleEndian.AppendUint16(want, OpcodeExSendManorList)
	want = binary.LittleEndian.AppendUint32(want, uint32(len(manorNames)))
	for i, name := range manorNames {
		want = binary.LittleEndian.AppendUint32(want, uint32(i+1))
		var w wire.Writer
		w.WriteString(name)
		want = append(want, w.Bytes()...)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("FrameExSendManorList() = % x, want % x", got, want)
	}
}
