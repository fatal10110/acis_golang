package serverpackets

import (
	"fmt"
	"math"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// The input rows of the probe (testdata/oracle/ProbeSimple.java ROWS,
// ROW_NAMES, STRS, DOUBLES, BLOBS): ten ints per row, distinct bytes per
// field in row "a", then negatives, zeros and the int limits. Packets take
// the first fields they need; the golden names are "Packet/row".
var oracleRows = []struct {
	name string
	v    [10]int32
}{
	{"a", [10]int32{0x01020304, 0x05060708, 0x090a0b0c, 0x0d0e0f10, 0x11121314, 0x15161718, 0x191a1b1c, 0x1d1e1f20, 0x21222324, 0x25262728}},
	{"neg", [10]int32{-1, -2, -3, -4, -5, -6, -7, -8, -9, -10}},
	{"zero", [10]int32{}},
	{"limits", [10]int32{math.MaxInt32, math.MinInt32, math.MaxInt32 - 1, math.MinInt32 + 1, 7, -7, 1, -1, 12345, -12345}},
}

var oracleStrings = []struct{ name, s string }{
	{"empty", ""},
	{"ascii", "Bo"},
	{"unicode", "Ünïcöde 日本語"},
	{"long", strings.Repeat("ab", 100)},
}

var oracleDoubles = []float64{0, 0.5, 1.1, -3.25, 1e10}

func oracleBlobs() []struct {
	name string
	data []byte
} {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	return []struct {
		name string
		data []byte
	}{{"empty", []byte{}}, {"small", []byte{1, 2, 3}}, {"bytes256", all}}
}

var oracleBools = []struct {
	name string
	b    bool
}{{"true", true}, {"false", false}}

func loc(x, y, z int32) location.Location {
	return location.Location{X: int(x), Y: int(y), Z: int(z)}
}

// oracleSimpleCases are the packets built from plain numbers, strings and
// byte slices.
func oracleSimpleCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for _, row := range oracleRows {
		v, n := row.v, "/"+row.name
		m["MyTargetSelected"+n] = func() wire.Frame { return FrameMyTargetSelected(v[0], int(v[1])) }
		m["TargetSelected"+n] = func() wire.Frame { return FrameTargetSelected(v[0], v[1], loc(v[2], v[3], v[4])) }
		m["AutoAttackStart"+n] = func() wire.Frame { return FrameAutoAttackStart(v[0]) }
		m["AutoAttackStop"+n] = func() wire.Frame { return FrameAutoAttackStop(v[0]) }
		m["StartRotation"+n] = func() wire.Frame { return FrameStartRotation(v[0], int(v[1]), int(v[2]), int(v[3])) }
		m["StopRotation"+n] = func() wire.Frame { return FrameStopRotation(v[0], int(v[1]), int(v[2])) }
		m["ChairSit"+n] = func() wire.Frame { return FrameChairSit(v[0], int(v[1])) }
		m["PetDelete"+n] = func() wire.Frame { return FramePetDelete(int(v[0]), v[1]) }
		m["ShortCutDelete"+n] = func() wire.Frame { return FrameShortCutDelete(v[0], v[1]) }
		m["ShowCalculator"+n] = func() wire.Frame { return FrameShowCalculator(v[0]) }
		m["ShowXMasSeal"+n] = func() wire.Frame { return FrameShowXMasSeal(v[0]) }
		m["ChooseInventoryItem"+n] = func() wire.Frame { return FrameChooseInventoryItem(v[0]) }
		m["ExRedSky"+n] = func() wire.Frame { return FrameExRedSky(v[0]) }
		m["CameraMode"+n] = func() wire.Frame { return FrameCameraMode(v[0]) }
		m["ExSetCompassZoneCode"+n] = func() wire.Frame { return FrameExSetCompassZoneCode(v[0]) }
		m["ExOlympiadMode"+n] = func() wire.Frame { return FrameExOlympiadMode(int(v[0])) }
		m["ExShowQuestMark"+n] = func() wire.Frame { return FrameExShowQuestMark(v[0]) }
		m["TutorialShowQuestionMark"+n] = func() wire.Frame { return FrameTutorialShowQuestionMark(v[0]) }
		m["TutorialEnableClientEvent"+n] = func() wire.Frame { return FrameTutorialEnableClientEvent(v[0]) }
		m["MagicSkillCanceled"+n] = func() wire.Frame { return FrameMagicSkillCanceled(v[0]) }
		m["SendTradeRequest"+n] = func() wire.Frame { return FrameSendTradeRequest(v[0]) }
		m["JoinPledge"+n] = func() wire.Frame { return FrameJoinPledge(v[0]) }
		m["JoinParty"+n] = func() wire.Frame { return FrameJoinParty(v[0]) }
		m["ExVariationCancelResult"+n] = func() wire.Frame { return FrameExVariationCancelResult(v[0]) }
		m["SetSummonRemainTime"+n] = func() wire.Frame { return FrameSetSummonRemainTime(v[0], v[1]) }
		m["ExConfirmVariationItem"+n] = func() wire.Frame { return FrameExConfirmVariationItem(v[0]) }
		m["ExConfirmVariationGemstone"+n] = func() wire.Frame { return FrameExConfirmVariationGemstone(v[0], v[1]) }
		m["ExConfirmVariationRefiner"+n] = func() wire.Frame {
			return FrameExConfirmVariationRefiner(v[0], v[1], v[2], v[3])
		}
		m["ExVariationResult"+n] = func() wire.Frame { return FrameExVariationResult(v[0], v[1], v[2]) }
		m["ExUseSharedGroupItem"+n] = func() wire.Frame {
			return FrameExUseSharedGroupItem(v[0], v[1], int(v[2]), int(v[3]))
		}
		m["ShortBuffStatusUpdate"+n] = func() wire.Frame { return FrameShortBuffStatusUpdate(v[0], v[1], v[2]) }
		m["RadarControl"+n] = func() wire.Frame {
			return FrameRadarControl(Radar{Show: v[0], Type: v[1], X: v[2], Y: v[3], Z: v[4]})
		}
		m["CharDeleteFail"+n] = func() wire.Frame { return FrameCharDeleteFail(CharDeleteFailReason(v[0])) }
		m["CharCreateFail"+n] = func() wire.Frame { return FrameCharCreateFail(CharCreateFailReason(v[0])) }
		m["SpecialCamera"+n] = func() wire.Frame {
			return FrameSpecialCamera(v[0], event.CameraShot{
				Distance: v[1], Yaw: v[2], Pitch: v[3], Time: v[4], Duration: v[5],
				Turn: v[6], Rise: v[7], Widescreen: v[8], Unknown: v[9],
			})
		}
		m["PledgePowerGradeList"+n] = func() wire.Frame {
			var counts [10]int
			for i, c := range v {
				counts[i] = int(c)
			}
			return FramePledgePowerGradeList(counts)
		}
		m["ExEnchantSkillInfo"+n] = func() wire.Frame {
			return FrameExEnchantSkillInfo(EnchantSkillInfo{ID: v[0], Level: v[1], SPCost: v[2], XPCost: int64(v[3]), Rate: v[4]})
		}
		m["ExShowScreenMessage"+n] = func() wire.Frame { return FrameExShowScreenMessage("msg", v[0]) }
		m["PlaySound/typed"+n] = func() wire.Frame { return FramePlaySoundAt(Sound{Type: v[0], File: "sound"}) }
		m["PlaySound/full"+n] = func() wire.Frame {
			return FramePlaySoundAt(Sound{
				Type: v[0], File: "sound", BindToObject: true, ObjectID: v[1],
				Location: loc(v[2], v[3], v[4]), Delay: v[5],
			})
		}
		for _, flag := range oracleBools {
			b, f := flag.b, n+"/"+flag.name
			m["EventTrigger"+f] = func() wire.Frame { return FrameEventTrigger(v[0], b) }
			m["ExFishingEnd"+f] = func() wire.Frame { return FrameExFishingEnd(v[0], b) }
		}
	}

	m["ExAutoSoulShot/off"] = func() wire.Frame { return FrameExAutoSoulShot(1463, false) }
	m["ExAutoSoulShot/on"] = func() wire.Frame { return FrameExAutoSoulShot(1463, true) }
	m["ExAutoSoulShot/negative-item"] = func() wire.Frame { return FrameExAutoSoulShot(-1, true) }

	for _, flag := range oracleBools {
		b, f := flag.b, "/"+flag.name
		m["ExDuelStart"+f] = func() wire.Frame { return FrameExDuelStart(b) }
		m["ExDuelEnd"+f] = func() wire.Frame { return FrameExDuelEnd(b) }
		m["ExDuelReady"+f] = func() wire.Frame { return FrameExDuelReady(b) }
		m["RestartResponse"+f] = func() wire.Frame { return FrameRestartResponse(b) }
		m["ExDuelAskStart"+f] = func() wire.Frame { return FrameExDuelAskStart("Bo", b) }
		m["SendTradeDone"+f] = func() wire.Frame { return FrameSendTradeDone(b) }
	}

	for i, d := range oracleDoubles {
		m[fmt.Sprintf("ExRegenMax/d%d", i)] = func() wire.Frame { return FrameExRegenMax(0x01020304, -7, d) }
	}

	for _, str := range oracleStrings {
		s, n := str.s, "/"+str.name
		m["AskJoinParty"+n] = func() wire.Frame { return FrameAskJoinParty(s, 3) }
		m["AskJoinAlly"+n] = func() wire.Frame { return FrameAskJoinAlly(0x01020304, s) }
		m["AskJoinPledge"+n] = func() wire.Frame { return FrameAskJoinPledge(0x01020304, s) }
		m["ExAskJoinMPCC"+n] = func() wire.Frame { return FrameExAskJoinMPCC(s) }
		m["ExAskJoinPartyRoom"+n] = func() wire.Frame { return FrameExAskJoinPartyRoom(s) }
		m["PledgeShowMemberListDelete"+n] = func() wire.Frame { return FramePledgeShowMemberListDelete(s) }
		m["ShowTownMap"+n] = func() wire.Frame { return FrameShowTownMap(s, -96622, 261660) }
		m["ShowBoard"+n] = func() wire.Frame { return FrameShowBoard("101", s) }
		m["ExShowScreenMessage/text-"+str.name] = func() wire.Frame { return FrameExShowScreenMessage(s, 5000) }
		m["PlaySound/file-"+str.name] = func() wire.Frame { return FramePlaySound(s) }
	}
	m["ShowBoard/no-page"] = func() wire.Frame { return FrameShowBoard("102", ShowBoardNoPage) }
	m["ShowBoard/fields"] = func() wire.Frame { return FrameShowBoardFields([]string{"a", "", "日"}) }

	for _, blob := range oracleBlobs() {
		data := blob.data
		m["AllyCrest/"+blob.name] = func() wire.Frame { return FrameAllyCrest(0x01020304, data) }
		m["ExPledgeCrestLarge/"+blob.name] = func() wire.Frame { return FrameExPledgeCrestLarge(0x01020304, data) }
	}

	reqs := []SkillRequirement{{Type: 4, ItemID: 0x01020304, Count: 0x05060708, Unknown: 0x090a0b0c}, {Type: -1, ItemID: -2, Count: -3, Unknown: -4}}
	m["AcquireSkillInfo/none"] = func() wire.Frame { return FrameAcquireSkillInfo(0x01020304, 5, 100000, 1, nil) }
	m["AcquireSkillInfo/two"] = func() wire.Frame { return FrameAcquireSkillInfo(-1, 0, math.MaxInt32, 2, reqs) }
	m["ExEnchantSkillInfo/two"] = func() wire.Frame {
		return FrameExEnchantSkillInfo(EnchantSkillInfo{
			ID: 0x01020304, Level: 101, SPCost: 5000, XPCost: 7, Rate: 82,
			Requirements: []EnchantSkillRequirement{
				{Type: 4, ItemID: 0x01020304, Count: 0x05060708, Unknown: 0x090a0b0c},
				{Type: -1, ItemID: -2, Count: -3, Unknown: -4},
			},
		})
	}

	// The named rejection reasons, against the reference's constants.
	for name, reason := range map[string]CharDeleteFailReason{
		"deletion-failed": CharDeleteFailReasonDeletionFailed,
		"clan-member":     CharDeleteFailReasonClanMemberMayNotDelete,
		"clan-leader":     CharDeleteFailReasonClanLeaderMayNotDelete,
	} {
		m["CharDeleteFail/"+name] = func() wire.Frame { return FrameCharDeleteFail(reason) }
	}
	for name, reason := range map[string]CharCreateFailReason{
		"creation-failed":       CharCreateFailReasonCreationFailed,
		"too-many-characters":   CharCreateFailReasonTooManyCharacters,
		"name-exists":           CharCreateFailReasonNameAlreadyExists,
		"16-eng-chars":          CharCreateFailReason16EngChars,
		"incorrect-name":        CharCreateFailReasonIncorrectName,
		"not-allowed":           CharCreateFailReasonCreateNotAllowed,
		"choose-another-server": CharCreateFailReasonChooseAnotherServer,
	} {
		m["CharCreateFail/"+name] = func() wire.Frame { return FrameCharCreateFail(reason) }
	}

	m["ActionFailed"] = FrameActionFailed
	m["ExMailArrived"] = FrameExMailArrived
	m["PartySmallWindowDeleteAll"] = FramePartySmallWindowDeleteAll
	m["PledgeShowMemberListDeleteAll"] = FramePledgeShowMemberListDeleteAll
	m["TradePressOwnOk"] = FrameTradePressOwnOk
	m["TradePressOtherOk"] = FrameTradePressOtherOk
	m["LeaveWorld"] = FrameLeaveWorld
	m["CharCreateOk"] = FrameCharCreateOk
	m["CharDeleteOk"] = FrameCharDeleteOk
	m["ExOlympiadMatchEnd"] = FrameExOlympiadMatchEnd
	m["ExPlayScene"] = FrameExPlayScene

	for _, npc := range []int32{12526, 12527, 12528, 12621, 12000, 0, -1} {
		m[fmt.Sprintf("Ride/mount-%d", npc)] = func() wire.Frame { return FrameRide(0x01020304, npc) }
	}
	m["Ride/dismount"] = func() wire.Frame { return FrameDismount(0x01020304) }
	return m
}

// oracleWorldCases are the packets the probe builds from a creature stand-in
// (testdata/oracle/ProbeWorld.java): id, x, y, z and heading are v[0..4], the
// target of a skill is the next row's creature.
func oracleWorldCases() map[string]func() wire.Frame {
	m := map[string]func() wire.Frame{}
	for r, row := range oracleRows {
		v, n := row.v, "/"+row.name
		w := oracleRows[(r+1)%len(oracleRows)].v
		at := loc(v[1], v[2], v[3])
		heading := int(v[4])

		m["MoveToLocation"+n] = func() wire.Frame { return FrameMoveToLocation(v[0], loc(v[5], v[6], v[7]), at) }
		m["MoveToLocation/explicit"+n] = func() wire.Frame { return FrameMoveToLocation(v[0], loc(v[8], v[9], v[5]), at) }
		m["MoveToPawn"+n] = func() wire.Frame { return FrameMoveToPawn(v[0], v[5], int(v[6]), at) }
		m["ValidateLocation"+n] = func() wire.Frame { return FrameValidateLocation(v[0], at, heading) }
		m["TeleportToLocation/slow"+n] = func() wire.Frame { return FrameTeleportToLocation(v[0], loc(v[5], v[6], v[7]), false) }
		m["TeleportToLocation/fast"+n] = func() wire.Frame { return FrameTeleportToLocation(v[0], loc(v[5], v[6], v[7]), true) }
		m["SocialAction"+n] = func() wire.Frame { return FrameSocialAction(v[0], v[5]) }
		m["DeleteObject"+n] = func() wire.Frame { return FrameDeleteObject(v[0], false) }
		m["DeleteObject/seated"+n] = func() wire.Frame { return FrameDeleteObject(v[0], true) }
		m["DeleteObject/standing"+n] = func() wire.Frame { return FrameDeleteObject(v[0], false) }
		m["TargetUnselected"+n] = func() wire.Frame { return FrameTargetUnselected(v[0], at) }
		m["Revive"+n] = func() wire.Frame { return FrameRevive(v[0]) }
		m["StopMove"+n] = func() wire.Frame { return FrameStopMove(v[0], at, heading) }
		m["Earthquake/npc"+n] = func() wire.Frame { return FrameEarthquake(at.X, at.Y, at.Z, v[5], v[6], true) }
		m["Earthquake/player"+n] = func() wire.Frame { return FrameEarthquake(at.X, at.Y, at.Z, v[5], v[6], false) }
		m["Earthquake/default"+n] = func() wire.Frame { return FrameEarthquake(at.X, at.Y, at.Z, v[5], v[6], false) }

		for _, wait := range []int32{0, 1, 2, 3, v[5]} {
			m[fmt.Sprintf("ChangeWaitType/%d%s", wait, n)] = func() wire.Frame {
				return FrameChangeWaitType(v[0], WaitType(wait), at)
			}
		}
		for _, running := range oracleBools {
			for _, swimming := range oracleBools {
				m[fmt.Sprintf("ChangeMoveType/%s-%s%s", running.name, swimming.name, n)] = func() wire.Frame {
					return FrameChangeMoveType(v[0], running.b, swimming.b)
				}
			}
		}

		caster := SkillCastObject{ObjectID: v[0], Location: at}
		target := SkillCastObject{ObjectID: w[0], Location: loc(w[1], w[2], w[3])}
		m["MagicSkillUse/hit"+n] = func() wire.Frame {
			return FrameMagicSkillUse(caster, target, v[5], v[6], int(v[7]), int(v[8]), false)
		}
		m["MagicSkillUse/crit"+n] = func() wire.Frame {
			return FrameMagicSkillUse(caster, target, v[5], v[6], int(v[7]), int(v[8]), true)
		}
		m["MagicSkillUse/self"+n] = func() wire.Frame {
			return FrameMagicSkillUse(caster, caster, v[5], v[6], int(v[7]), int(v[8]), false)
		}

		m["StatusUpdate"+n] = func() wire.Frame {
			return FrameStatusUpdate(v[0], []StatusAttribute{
				{Type: 1, Value: int(v[5])}, // level
				{Type: StatusCurrentHP, Value: int(v[6])},
				{Type: StatusKarma, Value: int(v[7])},
				{Type: StatusMaxCP, Value: int(v[8])},
			})
		}
		m["StatusUpdate/empty"+n] = func() wire.Frame { return FrameStatusUpdate(v[0], nil) }

		for _, flight := range []skill.Flight{skill.FlightThrowUp, skill.FlightThrowHorizontal, skill.FlightDummy, skill.FlightCharge} {
			m[fmt.Sprintf("FlyToLocation/%d%s", flight, n)] = func() wire.Frame {
				return FrameFlyToLocation(v[0], loc(v[5], v[6], v[7]), at, flight)
			}
		}
		for _, color := range []GaugeColor{GaugeBlue, GaugeRed, GaugeCyan, GaugeGreen} {
			m[fmt.Sprintf("SetupGauge/%d%s", color, n)] = func() wire.Frame { return FrameSetupGauge(color, int(v[0]), int(v[1])) }
			m[fmt.Sprintf("SetupGauge/single-%d%s", color, n)] = func() wire.Frame { return FrameSetupGauge(color, int(v[0]), int(v[0])) }
		}
	}
	for _, str := range oracleStrings {
		s := str.s
		m["TitleUpdate/"+str.name] = func() wire.Frame { return FrameTitleUpdate(0x01020304, s) }
	}
	return m
}
