import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.commons.mmocore.PacketBytes;
import net.sf.l2j.commons.pool.FakeDb;
import net.sf.l2j.gameserver.data.manager.CastleManager;
import net.sf.l2j.gameserver.data.manager.CastleManorManager;
import net.sf.l2j.gameserver.data.manager.ClanHallManager;
import net.sf.l2j.gameserver.data.xml.ItemData;
import net.sf.l2j.gameserver.enums.SiegeSide;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.instance.Monster;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.model.manor.CropProcure;
import net.sf.l2j.gameserver.model.manor.Seed;
import net.sf.l2j.gameserver.model.manor.SeedProduction;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
import net.sf.l2j.gameserver.model.residence.castle.Siege;
import net.sf.l2j.gameserver.model.residence.clanhall.ClanHallSiege;
import net.sf.l2j.commons.data.StatSet;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.SystemMessageId;
import net.sf.l2j.gameserver.network.serverpackets.*;
import net.sf.l2j.gameserver.taskmanager.GameTimeTaskManager;

/**
 * Packets that read the engine's item, clan, siege, castle, manor and character
 * graphs, through stand-ins for those graphs (stub/). Inputs are the shared
 * rows of ProbeSimple; the Go replay is packets_oracle_engine_test.go.
 */
final class ProbeEngine
{
	static void run() throws Exception
	{
		systemMessages();
		confirmDialogs();
		deaths();
		charSelected();
		charSelectInfo();
		manor();
	}

	private static SystemMessage sm(int id, int params)
	{
		return SystemMessage.getSystemMessage(new SystemMessageId(id, params));
	}

	private static void systemMessages() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			final String s = ProbeSimple.STRS[r];

			PacketProbe.emit("SystemMessage/none" + n, sm(v[0], 0));
			PacketProbe.emit("SystemMessage/string" + n, sm(v[0], 1).addString(s));
			PacketProbe.emit("SystemMessage/itemname" + n, sm(v[0], 1).addItemName(v[1]));
			PacketProbe.emit("SystemMessage/number" + n, sm(v[0], 1).addNumber(v[1]));
			PacketProbe.emit("SystemMessage/stringitemname" + n, sm(v[0], 2).addString(s).addItemName(v[1]));
			PacketProbe.emit("SystemMessage/stringnumber" + n, sm(v[0], 2).addString(s).addNumber(v[1]));
			PacketProbe.emit("SystemMessage/twonumbers" + n, sm(v[0], 2).addNumber(v[1]).addNumber(v[2]));
			PacketProbe.emit("SystemMessage/numberitemname" + n, sm(v[0], 2).addNumber(v[1]).addItemName(v[2]));
			PacketProbe.emit("SystemMessage/stringnumberitemname" + n, sm(v[0], 3).addString(s).addNumber(v[1]).addItemName(v[2]));
			PacketProbe.emit("SystemMessage/itemnamenumber" + n, sm(v[0], 2).addItemName(v[1]).addNumber(v[2]));
			PacketProbe.emit("SystemMessage/itemnameitemnumber" + n, sm(v[0], 2).addItemName(v[1]).addItemNumber(v[2]));
			PacketProbe.emit("SystemMessage/skillname" + n, sm(v[0], 1).addSkillName(v[1], v[2]));
			PacketProbe.emit("SystemMessage/stringskillname" + n, sm(v[0], 2).addString(s).addSkillName(v[1], v[2]));
			PacketProbe.emit("SystemMessage/params" + n, sm(v[0], 5).addString(s).addNumber(v[1]).addItemName(v[2]).addItemNumber(v[3]).addZoneName(new Location(v[4], v[5], v[6])));
			PacketProbe.emit("SystemMessage/fortid" + n, sm(v[0], 2).addFortId(v[1]).addString(s));
		}
		PacketProbe.emit("SystemMessage/sendString", SystemMessage.sendString("Bo"));
	}

	private static void confirmDialogs() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			final String s = ProbeSimple.STRS[r];

			// Summon-friend request: caster name and position, the countdown in ms, the caster's object id.
			PacketProbe.emit("ConfirmDlg/summon" + n, new ConfirmDlg(1842).addString(s).addZoneName(new Location(v[1], v[2], v[3])).addTime(v[4]).addRequesterId(v[0]));
			PacketProbe.emit("ConfirmDlg/resurrection" + n, new ConfirmDlg(1510).addString(s));
			PacketProbe.emit("ConfirmDlg/engage" + n, new ConfirmDlg(1983).addString(s));
		}
		// A zero countdown or requester is left out after the parameters.
		PacketProbe.emit("ConfirmDlg/summon/time-only", new ConfirmDlg(1842).addString("Bo").addZoneName(new Location(1, 2, 3)).addTime(30000));
		PacketProbe.emit("ConfirmDlg/summon/requester-only", new ConfirmDlg(1842).addString("Bo").addZoneName(new Location(1, 2, 3)).addRequesterId(0x01020304));
		PacketProbe.emit("ConfirmDlg/gate/open", new ConfirmDlg(1140));
		PacketProbe.emit("ConfirmDlg/gate/close", new ConfirmDlg(1141));
	}

	private static Player deadPlayer(int objectId)
	{
		final Player p = new Player();
		p.objectId = objectId;
		p.dead = true;
		return p;
	}

	/** A dead clan member with the given residences and siege flag. */
	private static Player clanMember(boolean clanHall, boolean castle, boolean flag)
	{
		final Player p = deadPlayer(0x01020304);
		p.clan = new Clan();
		p.clan.clanHall = clanHall;
		p.clan.castle = castle;
		p.clan.flag = flag ? new Object() : null;
		return p;
	}

	private static Siege siege(SiegeSide side)
	{
		final Siege s = new Siege();
		s.side = side;
		return s;
	}

	private static ClanHallSiege hallSiege(SiegeSide side)
	{
		final ClanHallSiege s = new ClanHallSiege();
		s.side = side;
		return s;
	}

	private static void die(String name, Creature p, Siege siege, ClanHallSiege hallSiege) throws Exception
	{
		CastleManager.getInstance().activeSiege = siege;
		ClanHallManager.getInstance().activeSiege = hallSiege;
		PacketProbe.emit("Die/" + name, new Die(p));
		CastleManager.getInstance().activeSiege = null;
		ClanHallManager.getInstance().activeSiege = null;
	}

	private static void deaths() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
			die("plain/" + ProbeSimple.ROW_NAMES[r], deadPlayer(ProbeSimple.ROWS[r][0]), null, null);

		final Player fixed = deadPlayer(0x01020304);
		fixed.accessLevel.allowFixedRes = true;
		die("fixedres", fixed, null, null);

		// Peace: the clan's own residences.
		die("clanhall", clanMember(true, false, false), null, null);
		die("castle", clanMember(false, true, false), null, null);
		die("clanhall-castle", clanMember(true, true, true), null, null);
		die("clan-flag-no-siege", clanMember(false, false, true), null, null);

		// A castle siege decides by the clan's side.
		die("siege-owner", clanMember(false, false, false), siege(SiegeSide.OWNER), null);
		die("siege-defender", clanMember(false, false, true), siege(SiegeSide.DEFENDER), null);
		die("siege-attacker-flag", clanMember(false, false, true), siege(SiegeSide.ATTACKER), null);
		die("siege-attacker-no-flag", clanMember(false, false, false), siege(SiegeSide.ATTACKER), null);
		die("siege-pending", clanMember(false, false, true), siege(SiegeSide.PENDING), null);
		die("siege-pending-castle", clanMember(false, true, false), siege(SiegeSide.PENDING), null);
		die("siege-attacker-clanhall", clanMember(true, false, true), siege(SiegeSide.ATTACKER), null);

		// A clan hall siege reports the siege headquarters only for an attacking, flagged clan.
		die("hall-siege-attacker-flag", clanMember(false, false, true), null, hallSiege(SiegeSide.ATTACKER));
		die("hall-siege-attacker-no-flag", clanMember(false, false, false), null, hallSiege(SiegeSide.ATTACKER));
		die("hall-siege-defender-flag", clanMember(false, false, true), null, hallSiege(SiegeSide.DEFENDER));
		die("hall-siege-residences", clanMember(true, true, true), null, hallSiege(SiegeSide.ATTACKER));
		// A castle siege takes precedence over a clan hall siege.
		die("siege-over-hall", clanMember(false, false, true), siege(SiegeSide.DEFENDER), hallSiege(SiegeSide.ATTACKER));
		// No clan: no siege is consulted.
		die("no-clan-siege", deadPlayer(0x01020304), siege(SiegeSide.ATTACKER), hallSiege(SiegeSide.ATTACKER));

		final Player everything = clanMember(true, true, true);
		everything.accessLevel.allowFixedRes = true;
		die("everything", everything, siege(SiegeSide.ATTACKER), null);

		final Monster sweepable = new Monster();
		sweepable.objectId = 0x01020304;
		sweepable.dead = true;
		sweepable.spoilState.sweepable = true;
		die("monster-sweep", sweepable, null, null);
		final Monster plain = new Monster();
		plain.objectId = 0x01020304;
		plain.dead = true;
		die("monster-plain", plain, null, null);
	}

	private static final int[] CLASS_INDEX =
	{
		0,
		18,
		53,
		88
	};

	private static void charSelected() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final Player p = charSelectedPlayer(r);
			// The stand-in answers what PlayerStatus.getINT..getWIT compute from
			// a class template's base attribute: never below 1 (Stats
			// cantBeNegative), and within the 1-99 the attribute bonus tables
			// cover, so the row's words fold into that range.
			p.playerStatus.intelligence = charSelectedAttr(v[0] + 1);
			p.playerStatus.strength = charSelectedAttr(v[1] + 2);
			p.playerStatus.constitution = charSelectedAttr(v[2] + 3);
			p.playerStatus.mentality = charSelectedAttr(v[3] + 4);
			p.playerStatus.dexterity = charSelectedAttr(v[4] + 5);
			p.playerStatus.wit = charSelectedAttr(v[5] + 6);
			GameTimeTaskManager.getInstance().gameTime = v[6] ^ v[7];
			PacketProbe.emit("CharSelected/" + ProbeSimple.ROW_NAMES[r], new CharSelected(p, v[2]));
		}
		charSelectedModified();
	}
	
	// Row a with attributes its live funcs moved off the class template's
	// (INT 21 + 3, STR 40 * 1.5, CON 43, MEN 25 + 4, DEX 30 - 2, WIT 11 + 4.75
	// truncated): getINT..getWIT answer the computed values.
	private static void charSelectedModified() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Player p = charSelectedPlayer(0);
		p.playerStatus.intelligence = 24;
		p.playerStatus.strength = 60;
		p.playerStatus.constitution = 43;
		p.playerStatus.mentality = 29;
		p.playerStatus.dexterity = 28;
		p.playerStatus.wit = 15;
		GameTimeTaskManager.getInstance().gameTime = v[6] ^ v[7];
		PacketProbe.emit("CharSelected/modified", new CharSelected(p, v[2]));
	}
	
	private static int charSelectedAttr(int x)
	{
		return 1 + (int) Math.floorMod((long) x - 1, 99L);
	}
	
	private static Player charSelectedPlayer(int r)
	{
		final int[] v = ProbeSimple.ROWS[r];
		final Player p = new Player();
		p.name = ProbeSimple.STRS[r];
		p.title = ProbeSimple.STRS[(r + 1) % ProbeSimple.STRS.length];
		p.objectId = v[0];
		p.clanId = v[1];
		p.appearance.sex = Sex.values()[v[2] & 1];
		p.race = ClassRace.values()[r];
		p.classId = ClassId.values()[CLASS_INDEX[r]];
		p.x = v[3];
		p.y = v[4];
		p.z = v[5];
		p.playerStatus.hp = v[6] + 0.5;
		p.playerStatus.mp = v[7] + 0.25;
		p.playerStatus.sp = v[8];
		p.playerStatus.exp = (long) v[9] * 1000003L + v[0];
		p.playerStatus.level = v[1] ^ v[2];
		p.karma = v[3] ^ v[4];
		p.pkKills = v[5] ^ v[6];
		return p;
	}

	// A character-list row: every field a function of the shared row v and the object id.
	private static void character(int objectId, String name, int[] v, int accessLevel, long lastAccess, int baseClass, int classId)
	{
		FakeDb.CHARACTERS.add(FakeDb.row("account_name", "acct", "obj_id", objectId, "char_name", name, "level", v[1], "maxhp", v[2], "curhp", v[3] + 0.5, "maxmp", v[4], "curmp", v[5] + 0.25,
			"karma", v[8], "pkkills", v[9], "pvpkills", v[0] ^ v[9], "face", v[6], "hairstyle", v[7], "haircolor", v[8] ^ v[7], "sex", v[3] & 1, "exp", (long) v[0] * 1000003L + v[1], "sp", v[4] ^ v[5],
			"clanid", v[2], "race", v[3] & 7, "x", v[5], "y", v[6], "z", v[7], "deletetime", 0L, "accesslevel", accessLevel, "lastaccess", lastAccess, "base_class", baseClass, "classid", classId));
	}

	static int paperdollObjectId(int[] v, int slot)
	{
		return v[slot % 10] + slot * 0x101;
	}

	static int paperdollItemId(int[] v, int slot)
	{
		return v[(slot + 3) % 10] - slot * 7;
	}

	// Equipped items: one per paperdoll slot 0..16; the right hand carries the enchant.
	private static void equip(int objectId, int[] v, int rhandEnchant, boolean augment, int augmentation)
	{
		for (int slot = 0; slot <= 16; slot++)
			FakeDb.ITEMS.add(FakeDb.row("owner_id", objectId, "object_id", paperdollObjectId(v, slot), "item_id", paperdollItemId(v, slot), "loc_data", slot, "enchant_level", slot == 7 ? rhandEnchant : slot));
		if (augment)
			FakeDb.AUGMENTATIONS.add(FakeDb.row("item_oid", paperdollObjectId(v, 7), "attributes", augmentation));
	}

	private static void charList(String name, String login, int session, int activeId) throws Exception
	{
		PacketProbe.emit(name, new CharSelectInfo(login, session, activeId), new GameClient());
	}

	private static void charSelectInfo() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			FakeDb.reset();
			character(v[0], ProbeSimple.STRS[r], v, 0, 0L, 10, 10);
			equip(v[0], v, v[9], paperdollObjectId(v, 7) > 0, v[8]);
			charList("CharSelectInfo/one" + n, "acct", v[2], 0);
		}

		final int[] a = ProbeSimple.ROWS[0];
		final int[] b = ProbeSimple.ROWS[1];
		final int[] c = ProbeSimple.ROWS[3];

		FakeDb.reset();
		charList("CharSelectInfo/empty", "acct", a[0], -1);

		// No explicit slot: the most recently played one is active, the first on a tie.
		FakeDb.reset();
		character(11, "One", a, 0, 50L, 10, 10);
		character(12, "Two", b, 0, 90L, 18, 18);
		character(13, "Three", c, 0, 90L, 53, 53);
		charList("CharSelectInfo/auto-recent", "acct", 7, -1);
		charList("CharSelectInfo/explicit-last", "acct", 7, 2);
		charList("CharSelectInfo/explicit-none", "acct", 7, 5);

		FakeDb.reset();
		character(11, "One", a, 0, 0L, 10, 10);
		character(12, "Two", b, 0, 0L, 18, 18);
		charList("CharSelectInfo/auto-never-played", "acct", 7, -1);

		// A banned character shows -1 where the deletion countdown goes.
		FakeDb.reset();
		character(11, "Banned", a, -1, 0L, 10, 10);
		charList("CharSelectInfo/banned", "acct", 7, 0);

		// On a subclass the experience, skill points and level come from the subclass row.
		FakeDb.reset();
		character(11, "Sub", a, 0, 0L, 10, 18);
		FakeDb.SUBCLASSES.add(FakeDb.row("char_obj_id", 11, "class_id", 18, "exp", 0x0102030405060708L, "sp", 777, "level", 44));
		charList("CharSelectInfo/subclass", "acct", 7, 0);

		// A character with no base class yet shows its class as the base.
		FakeDb.reset();
		character(11, "Base", a, 0, 0L, 0, 53);
		charList("CharSelectInfo/base-unset", "acct", 7, 0);

		// The augmentation of the equipped weapon; -1 reads as none.
		for (final int augmentation : new int[] { (8191 << 16) | 3, -1 })
		{
			FakeDb.reset();
			character(11, "Aug", a, 0, 0L, 10, 10);
			equip(11, a, 5, true, augmentation);
			charList("CharSelectInfo/augment/" + augmentation, "acct", 7, 0);
		}

		// The enchant byte is capped at 127.
		for (final int enchant : new int[] { 126, 127, 128, 200 })
		{
			FakeDb.reset();
			character(11, "Ench", a, 0, 0L, 10, 10);
			equip(11, a, enchant, false, 0);
			charList("CharSelectInfo/enchant/" + enchant, "acct", 7, 0);
		}

		// Only the character's own items are shown.
		FakeDb.reset();
		character(11, "Own", a, 0, 0L, 10, 10);
		equip(11, a, 3, false, 0);
		equip(99, b, 9, false, 0);
		charList("CharSelectInfo/own-items", "acct", 7, 0);
	}

	// ---- manor ----

	private static Item item(int id, int referencePrice)
	{
		final Item i = new Item();
		i.itemId = id;
		i.referencePrice = referencePrice;
		return i;
	}

	private static Seed seed(int cropId, int seedId, int matureId, int level, int reward1, int reward2, int castleId, boolean alternative, int seedsLimit, int cropsLimit)
	{
		return new Seed(new StatSet().set("id", cropId).set("seedId", seedId).set("matureId", matureId).set("level", level).set("reward1", reward1).set("reward2", reward2).set("castleId", castleId).set("isAlternative", alternative).set("seedsLimit", seedsLimit).set("cropsLimit", cropsLimit));
	}

	private static <T> List<T> list(T... items)
	{
		final List<T> l = new ArrayList<>();
		for (T t : items)
			l.add(t);
		return l;
	}

	private static void manor() throws Exception
	{
		// Three seeds, one per castle; the last one has no item reference price.
		ItemData.getInstance().templates.put(5016, item(5016, 7));
		ItemData.getInstance().templates.put(5073, item(5073, 51));
		ItemData.getInstance().templates.put(5012, item(5012, 13));
		ItemData.getInstance().templates.put(5068, item(5068, 96));
		final CastleManorManager m = CastleManorManager.getInstance();
		m.seeds.put(5016, seed(5073, 5016, 5103, 10, 1864, 1878, 1, false, 1000, 2000));
		m.seeds.put(5012, seed(5068, 5012, 5098, 20, 1865, 1879, 2, true, 500, 1500));
		m.seeds.put(5090, seed(5100, 5090, 5110, 30, 1866, 1880, 3, false, 300, 700));

		m.production.put(1, list(new SeedProduction(5016, 7, 30, 10), new SeedProduction(9999, 1, 3, 2)));
		m.productionNext.put(1, list(new SeedProduction(5016, 12, 33, 15)));
		m.production.put(2, new ArrayList<>());
		m.productionNext.put(2, list(new SeedProduction(5012, 4, 20, 9)));
		m.production.put(3, new ArrayList<>());
		m.productionNext.put(3, new ArrayList<>());
		m.production.put(4, list(new SeedProduction(5016, 0, 30, 10), new SeedProduction(5012, 3, 0, 4), new SeedProduction(5090, 2, 8, 6)));
		m.productionNext.put(4, new ArrayList<>());

		m.procure.put(1, list(new CropProcure(5073, 40, 2, 45, 50), new CropProcure(7777, 1, 1, 2, 3)));
		m.procureNext.put(1, list(new CropProcure(5073, 22, 1, 30, 55)));
		m.procure.put(2, list(new CropProcure(5073, 3, 1, 8, 31), new CropProcure(5068, 0, 2, 5, 70)));
		m.procureNext.put(2, new ArrayList<>());
		m.procure.put(3, list(new CropProcure(5073, 0, 1, 9, 12)));
		m.procureNext.put(3, new ArrayList<>());
		m.procure.put(4, new ArrayList<>());
		m.procureNext.put(4, new ArrayList<>());

		for (final boolean hide : new boolean[] { true, false })
		{
			final String h = hide ? "/hidden" : "/shown";
			PacketProbe.emit("ExShowSeedInfo/current" + h, new ExShowSeedInfo(1, false, hide));
			PacketProbe.emit("ExShowCropInfo/current" + h, new ExShowCropInfo(1, false, hide));
			PacketProbe.emit("ExShowManorDefaultInfo" + h, new ExShowManorDefaultInfo(hide));
		}
		PacketProbe.emit("ExShowSeedInfo/empty", new ExShowSeedInfo(3, false, false));
		PacketProbe.emit("ExShowCropInfo/empty", new ExShowCropInfo(4, false, true));

		// The next period is shown only once the manor is approved.
		m.approved = false;
		PacketProbe.emit("ExShowSeedInfo/next-unapproved", new ExShowSeedInfo(1, true, false));
		PacketProbe.emit("ExShowCropInfo/next-unapproved", new ExShowCropInfo(1, true, false));
		m.approved = true;
		PacketProbe.emit("ExShowSeedInfo/next-approved", new ExShowSeedInfo(1, true, true));
		PacketProbe.emit("ExShowCropInfo/next-approved", new ExShowCropInfo(1, true, true));

		// One seed per castle keeps the reference's set order out of the bytes.
		for (int castle = 1; castle <= 4; castle++)
		{
			PacketProbe.emit("ExShowSeedSetting/castle" + castle, new ExShowSeedSetting(castle));
			PacketProbe.emit("ExShowCropSetting/castle" + castle, new ExShowCropSetting(castle));
		}

		for (int castle = 1; castle <= 5; castle++)
			CastleManager.getInstance().castles.add(new Castle(castle));
		m.procure.put(5, new ArrayList<>());
		m.procureNext.put(5, new ArrayList<>());
		for (final int crop : new int[] { 5073, 5068, 5100 })
			PacketProbe.emit("ExShowProcureCropDetail/" + crop, new ExShowProcureCropDetail(crop));

		// The sell list: crops in the inventory, in the reference's map order.
		final Player seller = new Player();
		seller.adena = 900;
		final Item crop = item(5073, 51);
		final Item other = item(5068, 96);
		seller.inventory.byItemId.put(5073, new ItemInstance(268, crop, 12));
		PacketProbe.emit("ExShowSellCropList/one", new ExShowSellCropList(seller.inventory, 1));
		PacketProbe.emit("ExShowSellCropList/not-wanted", new ExShowSellCropList(seller.inventory, 3));
		seller.inventory.byItemId.put(5068, new ItemInstance(269, other, 1));
		seller.inventory.byItemId.put(5100, new ItemInstance(270, item(5100, 2), 0x7fffffff));
		PacketProbe.emit("ExShowSellCropList/three", new ExShowSellCropList(seller.inventory, 2));
		PacketProbe.emit("SellListProcure/one", new SellListProcure(seller, 1));
		PacketProbe.emit("SellListProcure/none", new SellListProcure(seller, 4));

		PacketProbe.emit("BuyListSeed/two", new BuyListSeed(1500, 1));
		PacketProbe.emit("BuyListSeed/filtered", new BuyListSeed(-5, 4));
		PacketProbe.emit("BuyListSeed/none", new BuyListSeed(0, 3));
		PacketProbe.emit("ExSendManorList", ExSendManorList.STATIC_PACKET);
	}
}
