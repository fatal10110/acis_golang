import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Packets built from plain numbers, strings and byte arrays. The input rows are
 * shared with the Go side (packets_oracle_cases_test.go): a row of ten ints with
 * distinct bytes per field, so a swapped or truncated field shows, then the
 * negative, zero and limit rows. Case names are "Packet/row".
 */
final class ProbeSimple
{
	static final String[] ROW_NAMES =
	{
		"a",
		"neg",
		"zero",
		"limits"
	};
	static final int[][] ROWS =
	{
		{ 0x01020304, 0x05060708, 0x090a0b0c, 0x0d0e0f10, 0x11121314, 0x15161718, 0x191a1b1c, 0x1d1e1f20, 0x21222324, 0x25262728 },
		{ -1, -2, -3, -4, -5, -6, -7, -8, -9, -10 },
		{ 0, 0, 0, 0, 0, 0, 0, 0, 0, 0 },
		{ Integer.MAX_VALUE, Integer.MIN_VALUE, Integer.MAX_VALUE - 1, Integer.MIN_VALUE + 1, 7, -7, 1, -1, 12345, -12345 }
	};

	static final String[] STR_NAMES =
	{
		"empty",
		"ascii",
		"unicode",
		"long"
	};
	static final String[] STRS =
	{
		"",
		"Bo",
		"Ünïcöde 日本語",
		"ab".repeat(100)
	};

	static final double[] DOUBLES =
	{
		0,
		0.5,
		1.1,
		-3.25,
		1e10
	};

	static final byte[][] BLOBS =
	{
		{},
		{ 1, 2, 3 },
		blob256()
	};
	static final String[] BLOB_NAMES =
	{
		"empty",
		"small",
		"bytes256"
	};

	private static byte[] blob256()
	{
		final byte[] b = new byte[256];
		for (int i = 0; i < b.length; i++)
			b[i] = (byte) i;
		return b;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ROWS.length; r++)
		{
			final int[] v = ROWS[r];
			final String n = "/" + ROW_NAMES[r];

			PacketProbe.emit("MyTargetSelected" + n, new MyTargetSelected(v[0], v[1]));
			PacketProbe.emit("TargetSelected" + n, new TargetSelected(v[0], v[1], v[2], v[3], v[4]));
			PacketProbe.emit("AutoAttackStart" + n, new AutoAttackStart(v[0]));
			PacketProbe.emit("AutoAttackStop" + n, new AutoAttackStop(v[0]));
			PacketProbe.emit("StartRotation" + n, new StartRotation(v[0], v[1], v[2], v[3]));
			PacketProbe.emit("StopRotation" + n, new StopRotation(v[0], v[1], v[2]));
			PacketProbe.emit("ChairSit" + n, new ChairSit(v[0], v[1]));
			PacketProbe.emit("PetDelete" + n, new PetDelete(v[0], v[1]));
			// The Go builder takes the slot and the page; the reference is handed the folded slot.
			PacketProbe.emit("ShortCutDelete" + n, new ShortCutDelete(v[0] + v[1] * 12));
			PacketProbe.emit("ShowCalculator" + n, new ShowCalculator(v[0]));
			PacketProbe.emit("ShowXMasSeal" + n, new ShowXMasSeal(v[0]));
			PacketProbe.emit("ChooseInventoryItem" + n, new ChooseInventoryItem(v[0]));
			PacketProbe.emit("ExRedSky" + n, new ExRedSky(v[0]));
			PacketProbe.emit("CameraMode" + n, new CameraMode(v[0]));
			PacketProbe.emit("ExSetCompassZoneCode" + n, new ExSetCompassZoneCode(v[0]));
			PacketProbe.emit("ExOlympiadMode" + n, new ExOlympiadMode(v[0]));
			PacketProbe.emit("ExShowQuestMark" + n, new ExShowQuestMark(v[0]));
			PacketProbe.emit("TutorialShowQuestionMark" + n, new TutorialShowQuestionMark(v[0]));
			PacketProbe.emit("TutorialEnableClientEvent" + n, new TutorialEnableClientEvent(v[0]));
			PacketProbe.emit("MagicSkillCanceled" + n, new MagicSkillCanceled(v[0]));
			PacketProbe.emit("SendTradeRequest" + n, new SendTradeRequest(v[0]));
			PacketProbe.emit("JoinPledge" + n, new JoinPledge(v[0]));
			PacketProbe.emit("JoinParty" + n, new JoinParty(v[0]));
			PacketProbe.emit("ExVariationCancelResult" + n, new ExVariationCancelResult(v[0]));
			PacketProbe.emit("SetSummonRemainTime" + n, new SetSummonRemainTime(v[0], v[1]));
			PacketProbe.emit("ExConfirmVariationItem" + n, new ExConfirmVariationItem(v[0]));
			PacketProbe.emit("ExConfirmVariationGemstone" + n, new ExConfirmVariationGemstone(v[0], v[1]));
			PacketProbe.emit("ExConfirmVariationRefiner" + n, new ExConfirmVariationRefiner(v[0], v[1], v[2], v[3]));
			PacketProbe.emit("ExVariationResult" + n, new ExVariationResult(v[0], v[1], v[2]));
			PacketProbe.emit("ExUseSharedGroupItem" + n, new ExUseSharedGroupItem(v[0], v[1], v[2], v[3]));
			PacketProbe.emit("ShortBuffStatusUpdate" + n, new ShortBuffStatusUpdate(v[0], v[1], v[2]));
			PacketProbe.emit("RadarControl" + n, new RadarControl(v[0], v[1], v[2], v[3], v[4]));
			PacketProbe.emit("CharDeleteFail" + n, new CharDeleteFail(v[0]));
			PacketProbe.emit("CharCreateFail" + n, new CharCreateFail(v[0]));
			PacketProbe.emit("SpecialCamera" + n, new SpecialCamera(v[0], v[1], v[2], v[3], v[4], v[5], v[6], v[7], v[8], v[9]));
			PacketProbe.emit("PledgePowerGradeList" + n, new PledgePowerGradeList(v));
			PacketProbe.emit("ExEnchantSkillInfo" + n, new ExEnchantSkillInfo(v[0], v[1], v[2], v[3], v[4]));
			PacketProbe.emit("ExShowScreenMessage" + n, new ExShowScreenMessage("msg", v[0]));
			PacketProbe.emit("PlaySound/typed" + n, new PlaySound(v[0], "sound"));
			PacketProbe.emit("PlaySound/full" + n, new PlaySound(v[0], "sound", true, v[1], new Location(v[2], v[3], v[4]), v[5]));

			for (final boolean flag : new boolean[] { true, false })
			{
				final String f = n + (flag ? "/true" : "/false");
				PacketProbe.emit("EventTrigger" + f, new EventTrigger(v[0], flag));
				PacketProbe.emit("ExFishingEnd" + f, new ExFishingEnd(flag, v[0]));
			}
		}

		// Reference calls take the int; the Go builder takes the boolean the int is 0 or 1 for.
		PacketProbe.emit("ExAutoSoulShot/off", new ExAutoSoulShot(1463, 0));
		PacketProbe.emit("ExAutoSoulShot/on", new ExAutoSoulShot(1463, 1));
		PacketProbe.emit("ExAutoSoulShot/negative-item", new ExAutoSoulShot(-1, 1));

		for (final boolean flag : new boolean[] { true, false })
		{
			final String f = flag ? "/true" : "/false";
			PacketProbe.emit("ExDuelStart" + f, new ExDuelStart(flag));
			PacketProbe.emit("ExDuelEnd" + f, new ExDuelEnd(flag));
			PacketProbe.emit("ExDuelReady" + f, new ExDuelReady(flag));
			PacketProbe.emit("RestartResponse" + f, new RestartResponse(flag));
			PacketProbe.emit("ExDuelAskStart" + f, new ExDuelAskStart("Bo", flag));
		}
		PacketProbe.emit("SendTradeDone/true", SendTradeDone.SUCCESS_STATIC_PACKET);
		PacketProbe.emit("SendTradeDone/false", SendTradeDone.FAIL_STATIC_PACKET);

		for (int i = 0; i < DOUBLES.length; i++)
			PacketProbe.emit("ExRegenMax/d" + i, new ExRegenMax(0x01020304, -7, DOUBLES[i]));

		for (int i = 0; i < STRS.length; i++)
		{
			final String s = STRS[i];
			final String n = "/" + STR_NAMES[i];
			PacketProbe.emit("AskJoinParty" + n, new AskJoinParty(s, 3));
			PacketProbe.emit("AskJoinAlly" + n, new AskJoinAlly(0x01020304, s));
			PacketProbe.emit("AskJoinPledge" + n, new AskJoinPledge(0x01020304, s));
			PacketProbe.emit("ExAskJoinMPCC" + n, new ExAskJoinMPCC(s));
			PacketProbe.emit("ExAskJoinPartyRoom" + n, new ExAskJoinPartyRoom(s));
			PacketProbe.emit("PledgeShowMemberListDelete" + n, new PledgeShowMemberListDelete(s));
			PacketProbe.emit("ShowTownMap" + n, new ShowTownMap(s, -96622, 261660));
			PacketProbe.emit("ShowBoard" + n, new ShowBoard(s, "101"));
			PacketProbe.emit("ExShowScreenMessage/text-" + STR_NAMES[i], new ExShowScreenMessage(s, 5000));
			PacketProbe.emit("PlaySound/file-" + STR_NAMES[i], new PlaySound(s));
		}
		PacketProbe.emit("ShowBoard/no-page", ShowBoard.STATIC_SHOWBOARD_102);
		PacketProbe.emit("ShowBoard/fields", new ShowBoard(java.util.List.of("a", "", "日")));

		for (int i = 0; i < BLOBS.length; i++)
		{
			PacketProbe.emit("AllyCrest/" + BLOB_NAMES[i], new AllyCrest(0x01020304, BLOBS[i]));
			PacketProbe.emit("ExPledgeCrestLarge/" + BLOB_NAMES[i], new ExPledgeCrestLarge(0x01020304, BLOBS[i]));
		}

		final AcquireSkillInfo noReqs = new AcquireSkillInfo(0x01020304, 5, 100000, 1);
		PacketProbe.emit("AcquireSkillInfo/none", noReqs);
		final AcquireSkillInfo reqs = new AcquireSkillInfo(-1, 0, Integer.MAX_VALUE, 2);
		reqs.addRequirement(4, 0x01020304, 0x05060708, 0x090a0b0c);
		reqs.addRequirement(-1, -2, -3, -4);
		PacketProbe.emit("AcquireSkillInfo/two", reqs);

		final ExEnchantSkillInfo enchantReqs = new ExEnchantSkillInfo(0x01020304, 101, 5000, 7, 82);
		enchantReqs.addRequirement(4, 0x01020304, 0x05060708, 0x090a0b0c);
		enchantReqs.addRequirement(-1, -2, -3, -4);
		PacketProbe.emit("ExEnchantSkillInfo/two", enchantReqs);

		PacketProbe.emit("CharDeleteFail/deletion-failed", CharDeleteFail.REASON_DELETION_FAILED);
		PacketProbe.emit("CharDeleteFail/clan-member", CharDeleteFail.REASON_YOU_MAY_NOT_DELETE_CLAN_MEMBER);
		PacketProbe.emit("CharDeleteFail/clan-leader", CharDeleteFail.REASON_CLAN_LEADERS_MAY_NOT_BE_DELETED);
		PacketProbe.emit("CharCreateFail/creation-failed", CharCreateFail.REASON_CREATION_FAILED);
		PacketProbe.emit("CharCreateFail/too-many-characters", CharCreateFail.REASON_TOO_MANY_CHARACTERS);
		PacketProbe.emit("CharCreateFail/name-exists", CharCreateFail.REASON_NAME_ALREADY_EXISTS);
		PacketProbe.emit("CharCreateFail/16-eng-chars", CharCreateFail.REASON_16_ENG_CHARS);
		PacketProbe.emit("CharCreateFail/incorrect-name", CharCreateFail.REASON_INCORRECT_NAME);
		PacketProbe.emit("CharCreateFail/not-allowed", CharCreateFail.REASON_CREATE_NOT_ALLOWED);
		PacketProbe.emit("CharCreateFail/choose-another-server", CharCreateFail.REASON_CHOOSE_ANOTHER_SVR);

		PacketProbe.emit("ActionFailed", ActionFailed.STATIC_PACKET);
		PacketProbe.emit("ExMailArrived", ExMailArrived.STATIC_PACKET);
		PacketProbe.emit("PartySmallWindowDeleteAll", PartySmallWindowDeleteAll.STATIC_PACKET);
		PacketProbe.emit("PledgeShowMemberListDeleteAll", PledgeShowMemberListDeleteAll.STATIC_PACKET);
		PacketProbe.emit("TradePressOwnOk", TradePressOwnOk.STATIC_PACKET);
		PacketProbe.emit("TradePressOtherOk", TradePressOtherOk.STATIC_PACKET);
		PacketProbe.emit("LeaveWorld", LeaveWorld.STATIC_PACKET);
		PacketProbe.emit("CharCreateOk", CharCreateOk.STATIC_PACKET);
		PacketProbe.emit("CharDeleteOk", CharDeleteOk.STATIC_PACKET);
		PacketProbe.emit("ExOlympiadMatchEnd", ExOlympiadMatchEnd.STATIC_PACKET);
		PacketProbe.emit("ExPlayScene", ExPlayScene.STATIC_PACKET);

		// Ride: the ride type follows the NPC id (striders 12526-12528, wyvern 12621) and the
		// class id is sent offset by 1000000.
		for (final int npc : new int[] { 12526, 12527, 12528, 12621, 12000, 0, -1 })
			PacketProbe.emit("Ride/mount-" + npc, new Ride(0x01020304, 1, npc));
		PacketProbe.emit("Ride/dismount", new Ride(0x01020304, 0, 0));
	}
}
