package clientpackets

import "testing"

func TestDecodeVariationRequests(t *testing.T) {
	target, err := DecodeRequestConfirmTargetItem([]byte{
		OpcodeExtended,
		0x29, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmTargetItem: %v", err)
	}
	if target != (RequestConfirmTargetItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmTargetItem = %+v, want ObjectID 1000", target)
	}

	refiner, err := DecodeRequestConfirmRefinerItem([]byte{
		OpcodeExtended,
		0x2a, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmRefinerItem: %v", err)
	}
	if refiner != (RequestConfirmRefinerItem{TargetObjectID: 1000, RefinerObjectID: 2000}) {
		t.Fatalf("DecodeRequestConfirmRefinerItem = %+v, want target 1000 refiner 2000", refiner)
	}

	gemstone, err := DecodeRequestConfirmGemStone([]byte{
		OpcodeExtended,
		0x2b, 0x00,
		0xe8, 0x03, 0x00, 0x00,
		0xd0, 0x07, 0x00, 0x00,
		0xb8, 0x0b, 0x00, 0x00,
		0x24, 0x00, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmGemStone: %v", err)
	}
	wantGemstone := RequestConfirmGemStone{
		TargetObjectID:   1000,
		RefinerObjectID:  2000,
		GemstoneObjectID: 3000,
		GemstoneCount:    36,
	}
	if gemstone != wantGemstone {
		t.Fatalf("DecodeRequestConfirmGemStone = %+v, want %+v", gemstone, wantGemstone)
	}

	cancel, err := DecodeRequestConfirmCancelItem([]byte{
		OpcodeExtended,
		0x2d, 0x00,
		0xe8, 0x03, 0x00, 0x00,
	})
	if err != nil {
		t.Fatalf("DecodeRequestConfirmCancelItem: %v", err)
	}
	if cancel != (RequestConfirmCancelItem{ObjectID: 1000}) {
		t.Fatalf("DecodeRequestConfirmCancelItem = %+v, want ObjectID 1000", cancel)
	}
}

func TestDecodeVariationRequestsShort(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x29, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x2a, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on short payload")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x2b, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on short payload")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x2d, 0x00, 1}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on short payload")
	}
}

func TestDecodeVariationRequestsWrongExtendedOpcode(t *testing.T) {
	if _, err := DecodeRequestConfirmTargetItem([]byte{OpcodeExtended, 0x2a, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmTargetItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmRefinerItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmRefinerItem: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmGemStone([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmGemStone: want error on wrong extended opcode")
	}
	if _, err := DecodeRequestConfirmCancelItem([]byte{OpcodeExtended, 0x29, 0x00, 0, 0, 0, 0}); err == nil {
		t.Fatal("DecodeRequestConfirmCancelItem: want error on wrong extended opcode")
	}
}
