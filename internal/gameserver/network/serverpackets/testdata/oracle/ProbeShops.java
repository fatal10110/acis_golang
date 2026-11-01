import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.data.xml.HennaData;
import net.sf.l2j.gameserver.data.xml.RecipeData;
import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.model.Macro;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.buylist.NpcBuyList;
import net.sf.l2j.gameserver.model.buylist.Product;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.model.multisell.Entry;
import net.sf.l2j.gameserver.model.multisell.Ingredient;
import net.sf.l2j.gameserver.model.multisell.ListContainer;
import net.sf.l2j.gameserver.model.records.Henna;
import net.sf.l2j.gameserver.model.records.MacroCmd;
import net.sf.l2j.gameserver.model.records.ManufactureItem;
import net.sf.l2j.gameserver.model.records.Recipe;
import net.sf.l2j.gameserver.model.trade.TradeItem;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Shop, trade, private store, recipe shop, macro and symbol windows. The
 * reference classes run over stand-ins for the item, inventory, trade and
 * crafting graphs (stub/); every field is a function of the shared row v through
 * {@link ProbeCharacters#f}, so the Go side (packets_oracle_shop_test.go) builds
 * the same values by name.
 */
final class ProbeShops
{
	static int f(int[] v, int i)
	{
		return ProbeCharacters.f(v, i);
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			shops(r, v, n);
			multiSell(v, n);
			trade(v, n);
			privateStores(r, v, n);
			recipes(r, v, n);
			macros(r, v, n);
			hennas(v, n);
		}
	}

	private static TradeItem tradeItem(int[] v, int kind, int count, int price)
	{
		return new TradeItem(ProbeItems.item(v, kind, false, false), count, price);
	}

	// ---- merchant windows ----

	private static void shops(int r, int[] v, String n) throws Exception
	{
		final NpcBuyList list = new NpcBuyList(f(v, 1));
		for (int kind = ProbeItems.WEAPON; kind <= ProbeItems.SIEGE; kind++)
		{
			final Item t = ProbeItems.template(kind, v);
			final int price = f(v, kind + 3);
			switch (kind % 3)
			{
				case 0:
					list.addProduct(new Product(t, price, -1, 0));
					break;
				case 1:
					list.addProduct(new Product(t, price, 5, f(v, kind + 4)));
					break;
				default:
					list.addProduct(new Product(t, price, 9, 3));
					break;
			}
		}
		Config.RATE_SIEGE_GUARDS_PRICE = ProbeSimple.DOUBLES[(r + 2) % ProbeSimple.DOUBLES.length];
		PacketProbe.emit("BuyList" + n, new BuyList(list, f(v, 0), ProbeSimple.DOUBLES[r % ProbeSimple.DOUBLES.length]));

		final List<ItemInstance> sell = new ArrayList<>(List.of(ProbeItems.items(v)));
		PacketProbe.emit("SellList" + n, new SellList(f(v, 2), sell));

		Config.WEAR_PRICE = f(v, 6);
		for (int expertise : new int[] { 0, 2, 5 })
			PacketProbe.emit("ShopPreviewList/exp" + expertise + n, new ShopPreviewList(list, f(v, 0), expertise));

		final java.util.Map<Paperdoll, Integer> worn = new java.util.EnumMap<>(Paperdoll.class);
		for (Paperdoll p : Paperdoll.values())
			if (p != Paperdoll.NULL)
				worn.put(p, f(v, p.getId()));
		PacketProbe.emit("ShopPreviewInfo" + n, new ShopPreviewInfo(worn));
	}

	private static Ingredient ingredient(int[] v, int kind, int count, int enchant)
	{
		final Item t = ProbeItems.template(kind, v);
		return new Ingredient(t.getItemId(), count, enchant, t);
	}

	private static Ingredient unknown(int count)
	{
		return new Ingredient(4242, count, 0, null);
	}

	private static void multiSell(int[] v, String n) throws Exception
	{
		final ListContainer list = new ListContainer(f(v, 0));
		for (int j = 0; j < 45; j++)
		{
			final List<Ingredient> products = new ArrayList<>();
			products.add(ingredient(v, j % 6, f(v, j), f(v, j + 1)));
			if (j % 3 == 0)
				products.add(unknown(f(v, j + 2)));
			final List<Ingredient> ingredients = new ArrayList<>();
			ingredients.add(ingredient(v, ProbeItems.ADENA, f(v, j + 3), 0));
			ingredients.add(ingredient(v, (j + 1) % 6, f(v, j + 4), f(v, j + 5)));
			if (j % 4 == 0)
				ingredients.add(unknown(f(v, j + 6)));
			list.getEntries().add(new Entry(ingredients, products));
		}
		PacketProbe.emit("MultiSellList/first" + n, new MultiSellList(list, 0));
		PacketProbe.emit("MultiSellList/last" + n, new MultiSellList(list, 40));
	}

	// ---- direct trade ----

	private static void trade(int[] v, String n) throws Exception
	{
		final Player player = new Player();
		ProbeItems.load(player, ProbeItems.items(v));
		player.activeTradeList = new net.sf.l2j.gameserver.model.trade.TradeList();
		player.activeTradeList.partner = new Player();
		player.activeTradeList.partner.objectId = f(v, 3);
		PacketProbe.emit("TradeStart" + n, new TradeStart(player));

		final int qty = f(v, 3);
		for (int kind : new int[] { ProbeItems.WEAPON, ProbeItems.ADENA })
		{
			final String k = kind == ProbeItems.WEAPON ? "/single" : "/stackable";
			final TradeItem ti = tradeItem(v, kind, f(v, kind + 1), f(v, kind + 2));
			PacketProbe.emit("TradeOwnAdd" + k + n, new TradeOwnAdd(ti, qty));
			PacketProbe.emit("TradeOtherAdd" + k + n, new TradeOtherAdd(ti, qty));
			PacketProbe.emit("TradeUpdate" + k + n, new TradeUpdate(ti, qty));
			PacketProbe.emit("TradeUpdate" + k + "/zero" + n, new TradeUpdate(ti, 0));
		}

		// The first row asks for the whole stack, so its availability is zero.
		final ItemInstance[] items = ProbeItems.items(v);
		final net.sf.l2j.gameserver.model.trade.TradeList offered = new net.sf.l2j.gameserver.model.trade.TradeList();
		offered.add(new TradeItem(items[ProbeItems.WEAPON], items[ProbeItems.WEAPON].count, 0));
		offered.add(new TradeItem(items[ProbeItems.QUEST], f(v, 5), 0));
		offered.add(new TradeItem(items[ProbeItems.ADENA], f(v, 6), 0));
		PacketProbe.emit("TradeItemUpdate" + n, new TradeItemUpdate(offered, player));
	}

	// ---- private stores ----

	private static void privateStores(int r, int[] v, String n) throws Exception
	{
		final Player viewer = new Player();
		viewer.adena = f(v, 2);
		final Player store = new Player();
		store.objectId = f(v, 0);
		store.name = "store";

		for (boolean packaged : new boolean[] { true, false })
		{
			final String p = packaged ? "/package" : "/plain";
			store.sellList.clear();
			store.sellList.packaged = packaged;
			store.sellList.add(tradeItem(v, ProbeItems.WEAPON, f(v, 1), f(v, 3)));
			store.sellList.add(tradeItem(v, ProbeItems.QUEST, f(v, 2), f(v, 4)));
			PacketProbe.emit("PrivateStoreListSell" + p + n, new PrivateStoreListSell(viewer, store));

			final Player owner = new Player();
			owner.objectId = f(v, 0);
			owner.adena = f(v, 2);
			owner.inventory.itemsToSell.add(tradeItem(v, ProbeItems.ARMOR, f(v, 4), ProbeItems.item(v, ProbeItems.ARMOR, false, false).getReferencePrice()));
			owner.inventory.itemsToSell.add(tradeItem(v, ProbeItems.JEWEL, f(v, 5), ProbeItems.item(v, ProbeItems.JEWEL, false, false).getReferencePrice()));
			owner.sellList.add(tradeItem(v, ProbeItems.WEAPON, f(v, 1), f(v, 3)));
			owner.sellList.add(tradeItem(v, ProbeItems.QUEST, f(v, 2), f(v, 4)));
			PacketProbe.emit("PrivateStoreManageListSell" + p + n, new PrivateStoreManageListSell(owner, packaged));
		}

		final TradeItem wanted1 = tradeItem(v, ProbeItems.WEAPON, f(v, 1), f(v, 3));
		wanted1.setQuantity(f(v, 5));
		final TradeItem wanted2 = tradeItem(v, ProbeItems.ADENA, f(v, 2), f(v, 4));
		wanted2.setQuantity(f(v, 6));
		store.buyList.available.add(wanted1);
		store.buyList.available.add(wanted2);
		PacketProbe.emit("PrivateStoreListBuy" + n, new PrivateStoreListBuy(viewer, store));

		final Player buyer = new Player();
		buyer.objectId = f(v, 0);
		buyer.adena = f(v, 2);
		buyer.inventory.uniqueItems.add(ProbeItems.item(v, ProbeItems.ARMOR, false, false));
		buyer.inventory.uniqueItems.add(ProbeItems.item(v, ProbeItems.JEWEL, false, false));
		buyer.buyList.add(new TradeItem(ProbeItems.template(ProbeItems.WEAPON, v), f(v, 1), f(v, 3), f(v, 2)));
		buyer.buyList.add(new TradeItem(ProbeItems.template(ProbeItems.QUEST, v), f(v, 4), f(v, 5), f(v, 6)));
		PacketProbe.emit("PrivateStoreManageListBuy" + n, new PrivateStoreManageListBuy(buyer));

		final Player titled = new Player();
		titled.objectId = f(v, 0);
		titled.sellList.title = ProbeSimple.STRS[r];
		titled.buyList.title = ProbeSimple.STRS[(r + 1) % 4];
		PacketProbe.emit("PrivateStoreMsgSell" + n, new PrivateStoreMsgSell(titled));
		PacketProbe.emit("PrivateStoreMsgBuy" + n, new PrivateStoreMsgBuy(titled));
	}

	// ---- recipes ----

	private static void recipes(int r, int[] v, String n) throws Exception
	{
		final Player crafter = new Player();
		crafter.objectId = f(v, 0);
		crafter.adena = f(v, 1);
		crafter.playerStatus.mp = f(v, 2);
		crafter.playerStatus.maxMp = f(v, 3);
		crafter.recipeBook.dwarven.add(new Recipe(f(v, 4), true));
		crafter.recipeBook.dwarven.add(new Recipe(f(v, 5), true));
		crafter.recipeBook.common.add(new Recipe(f(v, 6), false));
		crafter.manufactureList.add(new ManufactureItem(f(v, 4), f(v, 7), true));
		crafter.manufactureList.add(new ManufactureItem(f(v, 6), f(v, 8), false));
		crafter.manufactureList.add(new ManufactureItem(f(v, 5), f(v, 9), true));
		crafter.manufactureList.add(new ManufactureItem(999, f(v, 1), true));
		crafter.manufactureList.setStoreName(ProbeSimple.STRS[r]);

		crafter.dwarvenCraft = true;
		PacketProbe.emit("RecipeShopManageList/dwarven" + n, new RecipeShopManageList(crafter, true));
		PacketProbe.emit("RecipeShopManageList/common" + n, new RecipeShopManageList(crafter, false));
		crafter.dwarvenCraft = false;
		PacketProbe.emit("RecipeShopManageList/dwarven-without-skill" + n, new RecipeShopManageList(crafter, true));

		final Player buyer = new Player();
		buyer.adena = f(v, 2);
		PacketProbe.emit("RecipeShopSellList" + n, new RecipeShopSellList(buyer, crafter));
		PacketProbe.emit("RecipeShopItemInfo" + n, new RecipeShopItemInfo(crafter, f(v, 4)));
		PacketProbe.emit("RecipeShopMsg" + n, new RecipeShopMsg(crafter));
		PacketProbe.emit("RecipeBookItemList/dwarven" + n, new RecipeBookItemList(crafter, true));
		PacketProbe.emit("RecipeBookItemList/common" + n, new RecipeBookItemList(crafter, false));

		RecipeData.getInstance().recipes.put(f(v, 4), new Recipe(f(v, 4), true));
		PacketProbe.emit("RecipeItemMakeInfo/dwarven" + n, new RecipeItemMakeInfo(f(v, 4), crafter));
		PacketProbe.emit("RecipeItemMakeInfo/status" + n, new RecipeItemMakeInfo(f(v, 4), crafter, f(v, 7)));
		RecipeData.getInstance().recipes.clear();
		RecipeData.getInstance().recipes.put(f(v, 6), new Recipe(f(v, 6), false));
		PacketProbe.emit("RecipeItemMakeInfo/common" + n, new RecipeItemMakeInfo(f(v, 6), crafter));
		RecipeData.getInstance().recipes.clear();
	}

	// ---- macros ----

	private static void macros(int r, int[] v, String n) throws Exception
	{
		final MacroCmd[] cmds =
		{
			new MacroCmd(0, f(v, 4), f(v, 5), f(v, 6), ProbeSimple.STRS[(r + 3) % 4]),
			new MacroCmd(0, f(v, 7), f(v, 8), f(v, 9), ProbeSimple.STRS[r])
		};
		final Macro macro = new Macro(f(v, 0), f(v, 1), ProbeSimple.STRS[r], ProbeSimple.STRS[(r + 1) % 4], ProbeSimple.STRS[(r + 2) % 4], cmds);
		PacketProbe.emit("SendMacroList/macro" + n, new SendMacroList(f(v, 2), f(v, 3), macro));
		PacketProbe.emit("SendMacroList/none" + n, new SendMacroList(f(v, 2), f(v, 3), null));
		PacketProbe.emit("SendMacroList/empty" + n, new SendMacroList(f(v, 2), f(v, 3), new Macro(f(v, 0), f(v, 1), "", "", "", new MacroCmd[0])));
	}

	// ---- symbols ----

	private static Henna henna(int[] v, int k)
	{
		return new Henna(f(v, k), f(v, k + 1), f(v, k + 2), f(v, k + 3), f(v, k + 4), f(v, k + 5), f(v, k + 6), f(v, k + 7), f(v, k + 8), new int[] { 0 });
	}

	private static void hennas(int[] v, String n) throws Exception
	{
		final Player p = new Player();
		p.adena = f(v, 2);
		p.playerStatus.maxMp = 0;
		p.playerStatus.intelligence = f(v, 0);
		p.playerStatus.strength = f(v, 1);
		p.playerStatus.constitution = f(v, 2);
		p.playerStatus.mentality = f(v, 3);
		p.playerStatus.dexterity = f(v, 4);
		p.playerStatus.wit = f(v, 5);
		p.hennaList.maxSize = f(v, 3);
		p.hennaList.emptySlots = f(v, 4);
		for (int k = 0; k < 3; k++)
		{
			final Henna h = henna(v, k);
			HennaData.getInstance().hennas.add(h);
			p.hennaList.hennas.add(h);
			p.inventory.byItemId.put(h.dyeId(), new ItemInstance(0, ProbeItems.template(ProbeItems.ADENA, v), 1));
		}
		PacketProbe.emit("HennaEquipList" + n, new HennaEquipList(p));
		PacketProbe.emit("HennaUnequipList" + n, new HennaUnequipList(p));
		PacketProbe.emit("HennaItemInfo" + n, new HennaItemInfo(p.hennaList.hennas.get(0), p));
		PacketProbe.emit("HennaItemUnequipInfo" + n, new HennaItemUnequipInfo(p.hennaList.hennas.get(1), p));

		for (int i = 0; i < p.hennaList.stats.length; i++)
			p.hennaList.stats[i] = f(v, i);
		p.hennaList.unusable.add(p.hennaList.hennas.get(1).symbolId());
		PacketProbe.emit("HennaInfo" + n, new HennaInfo(p));
		HennaData.getInstance().hennas.clear();
	}
}
