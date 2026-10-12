package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

// darkCoda is Gludio's Dark Coda seed (5016) and crop (5073) as manors.xml
// and the item reference prices give it.
var darkCoda = manor.Seed{
	CropID: 5073, SeedID: 5016, MatureID: 5103, Level: 10, Reward1: 1864, Reward2: 1878,
	CastleID: 1, SeedsLimit: 1000, CropsLimit: 2000, SeedReferencePrice: 5, CropReferencePrice: 51,
}

func appendC(b []byte, v byte) []byte { return append(b, v) }

// appendRewards is the level and reward block every manor window row ends
// with.
func appendRewards(b []byte, level, reward1, reward2 int32) []byte {
	b = appendD(b, level)
	b = appendC(b, 1)
	b = appendD(b, reward1)
	b = appendC(b, 1)
	return appendD(b, reward2)
}

func checkFrame(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if !bytes.Equal(got, want) {
		t.Errorf("%s = % x\nwant % x", name, got, want)
	}
}

// Reference: ExShowSeedInfo.writeImpl. A row whose seed the table does
// not hold writes level 0 and reward items 0.
func TestFrameExShowSeedInfo(t *testing.T) {
	got := framePayload(t, FrameExShowSeedInfo(1, true, []ManorListing{
		{ID: 5016, Amount: 7, StartAmount: 10, Price: 30, Seed: darkCoda},
		{ID: 9999, Amount: 1, StartAmount: 2, Price: 3},
	}))
	want := appendH([]byte{OpcodeExtended}, 0x1c)
	want = appendC(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 0)
	want = appendD(want, 2)
	want = appendD(appendD(appendD(appendD(want, 5016), 7), 10), 30)
	want = appendRewards(want, 10, 1864, 1878)
	want = appendD(appendD(appendD(appendD(want, 9999), 1), 2), 3)
	want = appendRewards(want, 0, 0, 0)
	checkFrame(t, "FrameExShowSeedInfo", got, want)

	empty := framePayload(t, FrameExShowSeedInfo(3, false, nil))
	want = appendH([]byte{OpcodeExtended}, 0x1c)
	want = appendD(appendD(appendD(appendC(want, 0), 3), 0), 0)
	checkFrame(t, "FrameExShowSeedInfo(empty)", empty, want)
}

// Reference: ExShowCropInfo.writeImpl: the reward type is a byte after the
// price.
func TestFrameExShowCropInfo(t *testing.T) {
	got := framePayload(t, FrameExShowCropInfo(2, false, []ManorListing{
		{ID: 5073, Amount: 40, StartAmount: 100, Price: 50, Reward: 2, Seed: darkCoda},
		{ID: 8888, Amount: 0, StartAmount: 5, Price: 6, Reward: 1},
	}))
	want := appendH([]byte{OpcodeExtended}, 0x1d)
	want = appendC(want, 0)
	want = appendD(want, 2)
	want = appendD(want, 0)
	want = appendD(want, 2)
	want = appendC(appendD(appendD(appendD(appendD(want, 5073), 40), 100), 50), 2)
	want = appendRewards(want, 10, 1864, 1878)
	want = appendC(appendD(appendD(appendD(appendD(want, 8888), 0), 5), 6), 1)
	want = appendRewards(want, 0, 0, 0)
	checkFrame(t, "FrameExShowCropInfo", got, want)
}

// Reference: ExShowManorDefaultInfo.writeImpl.
func TestFrameExShowManorDefaultInfo(t *testing.T) {
	got := framePayload(t, FrameExShowManorDefaultInfo(true, []manor.Seed{darkCoda}))
	want := appendH([]byte{OpcodeExtended}, 0x1e)
	want = appendC(want, 1)
	want = appendD(want, 1)
	want = appendD(appendD(appendD(appendD(want, 5073), 10), 5), 51)
	want = appendC(want, 1)
	want = appendD(want, 1864)
	want = appendC(want, 1)
	want = appendD(want, 1878)
	checkFrame(t, "FrameExShowManorDefaultInfo", got, want)
}

// Reference: ExShowSeedSetting.writeImpl: the seed's reference price, then
// its bounds, (int)(5 * 0.6) = 3 and 5 * 10 = 50.
func TestFrameExShowSeedSetting(t *testing.T) {
	got := framePayload(t, FrameExShowSeedSetting(1, []ManorSetting{{
		Seed: darkCoda, Limit: 3000,
		Current: ManorPeriodSetting{StartAmount: 10, Price: 5},
	}}))
	want := appendH([]byte{OpcodeExtended}, 0x1f)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 5016)
	want = appendRewards(want, 10, 1864, 1878)
	want = appendD(appendD(appendD(appendD(want, 3000), 5), 3), 50)
	want = appendD(appendD(want, 10), 5)
	want = appendD(appendD(want, 0), 0)
	checkFrame(t, "FrameExShowSeedSetting", got, want)
}

// Reference: ExShowCropSetting.writeImpl: 0 where the seed window has the
// reference price, the crop's bounds (int)(51 * 0.6) = 30 and 510, and a
// reward byte after each period's price.
func TestFrameExShowCropSetting(t *testing.T) {
	got := framePayload(t, FrameExShowCropSetting(1, []ManorSetting{{
		Seed: darkCoda, Limit: 2000,
		Next: ManorPeriodSetting{StartAmount: 100, Price: 40, Reward: 2},
	}}))
	want := appendH([]byte{OpcodeExtended}, 0x20)
	want = appendD(want, 1)
	want = appendD(want, 1)
	want = appendD(want, 5073)
	want = appendRewards(want, 10, 1864, 1878)
	want = appendD(appendD(appendD(appendD(want, 2000), 0), 30), 510)
	want = appendC(appendD(appendD(want, 0), 0), 0)
	want = appendC(appendD(appendD(want, 100), 40), 2)
	checkFrame(t, "FrameExShowCropSetting", got, want)
}
