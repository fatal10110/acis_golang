import java.util.List;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.communitybbs.manager.BaseBBSManager;
import net.sf.l2j.gameserver.data.xml.PlayerData;
import net.sf.l2j.gameserver.enums.FailReason;
import net.sf.l2j.gameserver.enums.SayType;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.model.Augmentation;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.Summon;
import net.sf.l2j.gameserver.model.actor.instance.Fence;
import net.sf.l2j.gameserver.model.actor.template.PlayerTemplate;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.model.location.ObserverLocation;
import net.sf.l2j.gameserver.model.residence.clanhall.ClanHall;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * The remaining small families: html dialogs and their size limit, the community board page cuts,
 * fishing, storage limits, enchant and augmentation windows, dice, observer mode, clan hall
 * decoration, fences, npc chat, pet status, the sky, and the login and character creation packets.
 * The inputs are the shared rows of ProbeSimple (packets_oracle_misc_test.go reads the same);
 * case names are "Packet/variant/row".
 */
final class ProbeMisc
{
	// Headings a thrown die is probed at: the axes, the diagonals and odd values between them.
	static final int[] DICE_HEADINGS =
	{
		0,
		8192,
		12345,
		16384,
		32768,
		40000,
		49152,
		60000,
		65535
	};

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			final String s = ProbeSimple.STRS[r];
			for (int k = 0; k < ProbeSimple.STRS.length; k++)
				PacketProbe.emit("NpcHtmlMessage/" + ProbeSimple.STR_NAMES[k] + n, html(v[0], ProbeSimple.STRS[k], v[1]));
			fishing(v, n);

			final ItemInstance item = new ItemInstance(v[0], new Item(), 1);
			item.item.itemId = v[1];
			item.augmentation = new Augmentation(v[2]);
			PacketProbe.emit("ExConfirmCancelItem" + n, new ExConfirmCancelItem(item, v[3]));

			final Player roller = player(v[3]);
			roller.x = v[0];
			roller.y = v[1];
			roller.z = v[2];
			roller.heading = 40000;
			PacketProbe.emit("Dice" + n, new Dice(roller, v[4], v[5]));

			PacketProbe.emit("ObserverStart" + n, new ObserverStart(new ObserverLocation(0, v[0], v[1], v[2], v[3], v[4], 0, 0)));
			PacketProbe.emit("ObserverEnd" + n, new ObserverEnd(new Location(v[5], v[6], v[7])));

			final ClanHall hall = new ClanHall();
			hall.id = v[0];
			hall.depths.put(ClanHall.FUNC_RESTORE_HP, v[1]);
			hall.depths.put(ClanHall.FUNC_RESTORE_MP, v[2]);
			hall.depths.put(ClanHall.FUNC_RESTORE_EXP, v[3]);
			hall.depths.put(ClanHall.FUNC_TELEPORT, v[4]);
			hall.depths.put(ClanHall.FUNC_DECO_CURTAINS, v[5]);
			hall.depths.put(ClanHall.FUNC_SUPPORT_MAGIC, v[6]);
			hall.depths.put(ClanHall.FUNC_DECO_FIXTURES, v[7]);
			hall.depths.put(ClanHall.FUNC_CREATE_ITEM, v[8]);
			PacketProbe.emit("ClanHallDecoration" + n, new ClanHallDecoration(hall));

			final Fence fence = new Fence();
			fence.objectId = v[9];
			fence.type = v[1];
			fence.x = v[2];
			fence.y = v[3];
			fence.z = v[4];
			fence.sizeX = v[5];
			fence.sizeY = v[6];
			PacketProbe.emit("ExColosseumFenceInfo" + n, new ExColosseumFenceInfo(v[0], fence));

			PacketProbe.emit("NpcSay/all" + n, new NpcSay(v[1], SayType.ALL, s));
			PacketProbe.emit("NpcSay/shout" + n, new NpcSay(v[1], SayType.SHOUT, s));
			final Npc npc = new Npc();
			npc.objectId = v[0];
			npc.template.npcId = v[2];
			PacketProbe.emit("NpcSay/npc" + n, new NpcSay(npc, SayType.ALL, s));

			final Summon summon = new Summon();
			summon.summonType = v[0];
			PacketProbe.emit("PetStatusShow" + n, new PetStatusShow(summon));
		}

		final String a8191 = "a".repeat(8191);
		PacketProbe.emit("NpcHtmlMessage/limit-kept", html(7, a8191 + "a", 0));
		PacketProbe.emit("NpcHtmlMessage/limit-cut", html(7, a8191 + "aa", 0));
		PacketProbe.emit("NpcHtmlMessage/limit-wide-kept", html(7, a8191 + "\u65e5", 0));
		PacketProbe.emit("NpcHtmlMessage/limit-pair-cut", html(7, a8191 + "\ud83d\ude00", 0));

		for (int h : DICE_HEADINGS)
		{
			final Player roller = player(0x10203040);
			roller.x = -84318;
			roller.y = 244579;
			roller.z = -3730;
			roller.heading = h;
			PacketProbe.emit("Dice/heading-" + h, new Dice(roller, 4625, 6));
		}

		storage("human", 90, 12, 15, 17, 18, 21, 23);
		storage("dwarf", 117, 13, 15, 18, 19, 21, 23);

		PacketProbe.emit("EnchantResult/success", EnchantResult.SUCCESS);
		PacketProbe.emit("EnchantResult/broken-crystals", EnchantResult.UNK_RESULT_1);
		PacketProbe.emit("EnchantResult/cancelled", EnchantResult.CANCELLED);
		PacketProbe.emit("EnchantResult/unsuccess", EnchantResult.UNSUCCESS);
		PacketProbe.emit("EnchantResult/broken-no-crystals", EnchantResult.UNK_RESULT_4);
		PacketProbe.emit("ExShowVariationMakeWindow", ExShowVariationMakeWindow.STATIC_PACKET);
		PacketProbe.emit("ExShowVariationCancelWindow", ExShowVariationCancelWindow.STATIC_PACKET);
		PacketProbe.emit("ExShowQuestInfo", ExShowQuestInfo.STATIC_PACKET);
		PacketProbe.emit("SunRise", SunRise.STATIC_PACKET);
		PacketProbe.emit("SunSet", SunSet.STATIC_PACKET);

		login();
		board();
	}

	static Player player(int objectId)
	{
		final Player p = new Player();
		p.objectId = objectId;
		return p;
	}

	private static NpcHtmlMessage html(int objectId, String text, int itemId)
	{
		final NpcHtmlMessage html = new NpcHtmlMessage(objectId);
		html.setHtml(text);
		html.setItemId(itemId);
		return html;
	}

	private static void fishing(int[] v, String n) throws Exception
	{
		final Player p = player(v[0]);
		final Location bait = new Location(v[2], v[3], v[4]);
		for (int k = 0; k < 4; k++)
		{
			final boolean night = (k & 1) != 0;
			final boolean ranked = (k & 2) != 0;
			Config.ALLOW_FISH_CHAMPIONSHIP = ranked;
			PacketProbe.emit("ExFishingStart/" + (night ? "night" : "day") + (ranked ? "-ranked" : "") + n, new ExFishingStart(p, v[1], bait, night));
		}
		Config.ALLOW_FISH_CHAMPIONSHIP = false;
		PacketProbe.emit("ExFishingStartCombat" + n, new ExFishingStartCombat(p, v[1], v[2], v[3], v[4], v[5]));
		PacketProbe.emit("ExFishingHpRegen" + n, new ExFishingHpRegen(p, v[1], v[2], v[3], v[4], v[5], v[6], v[7]));
	}

	// The limits a human and a dwarf of the Go side's storage fixture hold (each its own value).
	private static void storage(String name, int inventory, int warehouse, int freight, int sell, int buy, int dwarfRecipe, int commonRecipe) throws Exception
	{
		final Player p = player(1);
		p.getStatus().inventoryLimit = inventory;
		p.getStatus().wareHouseLimit = warehouse;
		p.getStatus().freightLimit = freight;
		p.getStatus().privateSellStoreLimit = sell;
		p.getStatus().privateBuyStoreLimit = buy;
		p.getStatus().dwarfRecipeLimit = dwarfRecipe;
		p.getStatus().commonRecipeLimit = commonRecipe;
		PacketProbe.emit("ExStorageMaxCount/" + name, new ExStorageMaxCount(p));
	}

	private static void login() throws Exception
	{
		PacketProbe.emit("AuthLoginFail", new AuthLoginFail(FailReason.SYSTEM_ERROR_LOGIN_LATER));

		final byte[] key = new byte[16];
		for (int i = 0; i < key.length; i++)
			key[i] = (byte) (0xf1 + i);
		Config.USE_BLOWFISH_CIPHER = true;
		PacketProbe.emit("VersionCheck/cipher", new VersionCheck(key));
		Config.USE_BLOWFISH_CIPHER = false;
		PacketProbe.emit("VersionCheck/plain", new VersionCheck(key));
		Config.USE_BLOWFISH_CIPHER = true;

		// Each root class gets its own base stats; the packet reads them in its own order.
		final ClassId[] roots =
		{
			ClassId.HUMAN_FIGHTER,
			ClassId.HUMAN_MYSTIC,
			ClassId.ELVEN_FIGHTER,
			ClassId.ELVEN_MYSTIC,
			ClassId.DARK_FIGHTER,
			ClassId.DARK_MYSTIC,
			ClassId.ORC_FIGHTER,
			ClassId.ORC_MYSTIC,
			ClassId.DWARVEN_FIGHTER
		};
		for (int k = 0; k < roots.length; k++)
		{
			final PlayerTemplate t = new PlayerTemplate();
			t.classId = roots[k];
			t.str = 10 + k;
			t.dex = 20 + k;
			t.con = 30 + k;
			t.intelligence = 40 + k;
			t.wit = 50 + k;
			t.men = 60 + k;
			PlayerData.getInstance().templates.put(roots[k].getId(), t);
		}
		PacketProbe.emit("NewCharacterSuccess", NewCharacterSuccess.STATIC_PACKET);
	}

	// Community board pages go out cut into parts by the board's own send helpers.
	private static void board() throws Exception
	{
		final String a = "a".repeat(4090);
		final String b = "b".repeat(4090);
		final String c = "c".repeat(10);
		page("short", "<html/>");
		page("one-part", a);
		page("two-parts", a + c);
		page("three-parts", a + b + c);
		page("too-long", a + b + "c".repeat(4090));
		page("wide", "a".repeat(4089) + "\u00e9");
		page("straddle", "a".repeat(4089) + "\ud83d\ude00" + "b");

		Player p = new Player();
		Board.edit("a".repeat(8179), p);
		sent("ShowBoardEdit/fits", p.sent);
		p = new Player();
		Board.edit("a".repeat(8180), p);
		sent("ShowBoardEdit/too-long", p.sent);
	}

	private static void page(String name, String html) throws Exception
	{
		final Player p = new Player();
		BaseBBSManager.separateAndSend(html, p);
		sent("ShowBoardPage/" + name, p.sent);
	}

	private static void sent(String name, List<L2GameServerPacket> packets) throws Exception
	{
		for (int i = 0; i < packets.size(); i++)
			PacketProbe.emit(name + "/" + i, packets.get(i));
	}

	// Reaches the board's protected edit-page helper.
	private static final class Board extends BaseBBSManager
	{
		static void edit(String html, Player player)
		{
			send1001(html, player);
		}
	}
}
