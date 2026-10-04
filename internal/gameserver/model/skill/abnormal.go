package skill

import "fmt"

// AbnormalStealth is the stealth abnormal visual: an invisible game master
// sees itself, and its summon, drawn with it.
const AbnormalStealth = 0x100000

// AbnormalHold2 is the second hold visual: a player an admin changes the
// class or sex of is drawn held while it is off the grid.
const AbnormalHold2 = 0x000800

// abnormalEffectMasks maps an effect template's abnormal="..." name to the
// client's abnormal-visual bit carried in CharInfo, UserInfo, NpcInfo and
// the summon info packets. "null" is the explicit no-visual value.
var abnormalEffectMasks = map[string]int{
	"null":          0,
	"bleeding":      0x000001,
	"poison":        0x000002,
	"redcircle":     0x000004,
	"ice":           0x000008,
	"wind":          0x000010,
	"fear":          0x000020,
	"stun":          0x000040,
	"sleep":         0x000080,
	"mute":          0x000100,
	"root":          0x000200,
	"hold1":         0x000400,
	"hold2":         AbnormalHold2,
	"unknown13":     0x001000,
	"bighead":       0x002000,
	"flame":         0x004000,
	"changetexture": 0x008000,
	"grow":          0x010000,
	"floatroot":     0x020000,
	"dancestun":     0x040000,
	"firerootstun":  0x080000,
	"stealth":       AbnormalStealth,
	"imprison1":     0x200000,
	"imprison2":     0x400000,
	"magiccircle":   0x800000,
}

// ParseAbnormalEffect resolves an effect template's abnormal visual name to
// its client bitmask. Names are case-sensitive; an unknown name is an error.
func ParseAbnormalEffect(name string) (int, error) {
	mask, ok := abnormalEffectMasks[name]
	if !ok {
		return 0, fmt.Errorf("skill: unknown abnormal effect %q", name)
	}
	return mask, nil
}
