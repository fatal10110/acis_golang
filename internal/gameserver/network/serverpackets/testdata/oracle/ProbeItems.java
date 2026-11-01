import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;

import net.sf.l2j.gameserver.enums.items.CrystalType;
import net.sf.l2j.gameserver.enums.items.ItemState;
import net.sf.l2j.gameserver.model.Augmentation;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.Summon;
import net.sf.l2j.gameserver.model.actor.instance.Pet;
import net.sf.l2j.gameserver.model.item.instance.ItemInfo;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Item packets: the inventory lists and updates, ground items, freight and
 * warehouse lists. The reference classes run over stand-ins for the item and
 * inventory graphs (stub/); every field is a function of the shared row v through
 * {@link ProbeCharacters#f}, so the Go side (packets_oracle_item_test.go) builds
 * the same items by name.
 */
final class ProbeItems
{
	static final int WEAPON = 0;
	static final int ARMOR = 1;
	static final int JEWEL = 2;
	static final int QUEST = 3;
	static final int ADENA = 4;
	static final int SIEGE = 5;

	static int f(int[] v, int i)
	{
		return ProbeCharacters.f(v, i);
	}

	static Item template(int kind, int[] v)
	{
		final Item i = new Item();
		i.referencePrice = f(v, kind + 9);
		switch (kind)
		{
			case WEAPON:
				i.itemId = 2368;
				i.type1 = 0;
				i.type2 = 0;
				i.bodyPart = 0x4000;
				i.crystalType = CrystalType.C;
				break;
			case ARMOR:
				i.itemId = 1146;
				i.type1 = 1;
				i.type2 = 1;
				i.bodyPart = 0x0400;
				i.duration = 60;
				i.crystalType = CrystalType.B;
				break;
			case JEWEL:
				i.itemId = 112;
				i.type1 = 0;
				i.type2 = 2;
				i.bodyPart = 0x0008;
				i.crystalType = CrystalType.D;
				break;
			case QUEST:
				i.itemId = 1000;
				i.type1 = 4;
				i.type2 = 3;
				i.stackable = true;
				break;
			case SIEGE:
				i.itemId = 4000;
				i.type1 = 4;
				i.type2 = 5;
				i.stackable = true;
				break;
			default:
				i.itemId = 57;
				i.type1 = 4;
				i.type2 = 4;
				i.stackable = true;
				break;
		}
		return i;
	}

	// The item of the given kind that row v describes. Position i rotates the
	// row so the five items of a list differ.
	static ItemInstance item(int[] v, int kind, boolean equipped, boolean augmented)
	{
		final int i = kind;
		final ItemInstance it = new ItemInstance(f(v, i), template(kind, v), f(v, i + 1));
		it.enchantLevel = f(v, i + 2);
		it.customType1 = f(v, i + 3);
		it.customType2 = f(v, i + 4);
		it.manaLeft = f(v, i + 5);
		it.equipped = equipped;
		if (augmented)
			it.augmentation = new Augmentation(f(v, i + 6));
		it.x = f(v, i + 7);
		it.y = f(v, i + 8);
		it.z = f(v, i + 9);
		return it;
	}

	// The five-item list the Go side mirrors: a worn augmented weapon, a worn shadow
	// armor, an augmented jewel, a quest stack and adena.
	static ItemInstance[] items(int[] v)
	{
		return new ItemInstance[]
		{
			item(v, WEAPON, true, true),
			item(v, ARMOR, true, false),
			item(v, JEWEL, false, true),
			item(v, QUEST, false, false),
			item(v, ADENA, false, false)
		};
	}

	static void load(Player player, ItemInstance[] items)
	{
		for (ItemInstance it : items)
			player.inventory.items.add(it);
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			lists(r, v, n);
			updates(v, n);
			ground(v, n);
			freight(v, n);
		}
		final Player empty = new Player();
		PacketProbe.emit("ItemList/empty", new ItemList(empty, true));
		PacketProbe.emit("PetItemList/empty", new PetItemList(new Summon()));
	}

	private static void lists(int r, int[] v, String n) throws Exception
	{
		final ItemInstance[] items = items(v);
		final Player player = new Player();
		load(player, items);
		PacketProbe.emit("ItemList/show" + n, new ItemList(player, true));
		PacketProbe.emit("ItemList/hide" + n, new ItemList(player, false));

		final Summon summon = new Summon();
		for (ItemInstance it : items)
			summon.inventory.items.add(it);
		PacketProbe.emit("PetItemList" + n, new PetItemList(summon));

		player.name = ProbeSimple.STRS[r];
		player.playerStatus.inventoryLimit = f(v, 7);
		PacketProbe.emit("GMViewItemList/player" + n, new GMViewItemList(player));

		final Pet pet = new Pet();
		pet.name = ProbeSimple.STRS[(r + 1) % 4];
		pet.inventoryLimit = f(v, 8);
		for (ItemInstance it : items)
			pet.inventory.items.add(it);
		PacketProbe.emit("GMViewItemList/pet" + n, new GMViewItemList(pet));
	}

	private static void updates(int[] v, String n) throws Exception
	{
		final ItemInstance[] items = items(v);
		final ItemState[] states =
		{
			ItemState.ADDED,
			ItemState.MODIFIED,
			ItemState.REMOVED,
			ItemState.MODIFIED,
			ItemState.ADDED
		};
		final Player player = new Player();
		final Summon summon = new Summon();
		for (int i = 0; i < items.length; i++)
		{
			player.inventory.updateList.add(new ItemInfo(items[i], states[i]));
			summon.inventory.updateList.add(new ItemInfo(items[i], states[i]));
		}
		PacketProbe.emit("InventoryUpdate" + n, new InventoryUpdate(player));
		PacketProbe.emit("PetInventoryUpdate" + n, new PetInventoryUpdate(summon));
	}

	private static void ground(int[] v, String n) throws Exception
	{
		final ItemInstance stack = item(v, ADENA, false, false);
		final ItemInstance single = item(v, WEAPON, false, false);
		PacketProbe.emit("SpawnItem/stackable" + n, new SpawnItem(stack));
		PacketProbe.emit("SpawnItem/single" + n, new SpawnItem(single));
		PacketProbe.emit("DropItem/stackable" + n, new DropItem(stack, f(v, 3)));
		PacketProbe.emit("DropItem/single" + n, new DropItem(single, f(v, 3)));
		PacketProbe.emit("GetItem" + n, new GetItem(stack, f(v, 3)));
	}

	private static void freight(int[] v, String n) throws Exception
	{
		final ItemInstance[] items = items(v);
		final Map<Integer, String> recipients = new LinkedHashMap<>();
		for (int i = 0; i < 3; i++)
			recipients.put(f(v, i) * 3 + i, ProbeSimple.STRS[i + 1]);
		PacketProbe.emit("PackageToList" + n, new PackageToList(recipients));

		final GameClient client = new GameClient();
		client.player = new Player();
		client.player.adena = f(v, 2);
		PacketProbe.emit("PackageSendableList" + n, new PackageSendableList(items, f(v, 1)), client);

		final Player player = new Player();
		player.adena = f(v, 2);
		load(player, items);
		for (ItemInstance it : items)
			player.warehouse.items.add(it);
		for (int type = 1; type <= 4; type++)
		{
			PacketProbe.emit("WarehouseDepositList/" + type + n, new WarehouseDepositList(player, type));
			PacketProbe.emit("WarehouseWithdrawList/" + type + n, new WarehouseWithdrawList(player, type));
		}
	}
}
