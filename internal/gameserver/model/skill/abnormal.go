package skill

import "fmt"

// Client abnormal-visual bits the server sets itself, outside an effect
// template's abnormal="..." name.
const (
	AbnormalFear  = 0x000020
	AbnormalStun  = 0x000040
	AbnormalSleep = 0x000080
	AbnormalMute  = 0x000100
	AbnormalRoot  = 0x000200
	AbnormalHold1 = 0x000400
	// AbnormalHold2 is the second hold visual, also shown by a petrified
	// creature and by a player an admin changes the class or sex of while
	// it is off the grid.
	AbnormalHold2     = 0x000800
	AbnormalBigHead   = 0x002000
	AbnormalGrow      = 0x010000
	AbnormalFloatRoot = 0x020000
	// AbnormalStealth is the stealth visual: an invisible game master sees
	// itself, and its summon, drawn with it.
	AbnormalStealth     = 0x100000
	AbnormalMagicCircle = 0x800000
)

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
	"fear":          AbnormalFear,
	"stun":          AbnormalStun,
	"sleep":         AbnormalSleep,
	"mute":          AbnormalMute,
	"root":          AbnormalRoot,
	"hold1":         AbnormalHold1,
	"hold2":         AbnormalHold2,
	"unknown13":     0x001000,
	"bighead":       AbnormalBigHead,
	"flame":         0x004000,
	"changetexture": 0x008000,
	"grow":          AbnormalGrow,
	"floatroot":     AbnormalFloatRoot,
	"dancestun":     0x040000,
	"firerootstun":  0x080000,
	"stealth":       AbnormalStealth,
	"imprison1":     0x200000,
	"imprison2":     0x400000,
	"magiccircle":   AbnormalMagicCircle,
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
