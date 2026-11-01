import java.util.function.Consumer;

import net.sf.l2j.Config;
import net.sf.l2j.gameserver.data.manager.CursedWeaponManager;
import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.enums.TeamType;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.actors.ClassRace;
import net.sf.l2j.gameserver.enums.actors.MoveType;
import net.sf.l2j.gameserver.enums.actors.OperateType;
import net.sf.l2j.gameserver.enums.actors.Sex;
import net.sf.l2j.gameserver.model.actor.Boat;
import net.sf.l2j.gameserver.model.actor.container.player.BoatInfo;
import net.sf.l2j.gameserver.model.actor.instance.Cubic;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.network.GameClient;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.Summon;
import net.sf.l2j.gameserver.model.actor.instance.Pet;
import net.sf.l2j.gameserver.model.actor.instance.Servitor;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Packets that describe a character, an npc or a summon to the client: UserInfo,
 * CharInfo, NpcInfo, SummonInfo, PetInfo, PetStatusUpdate and ServerObjectInfo. The
 * reference classes run over plain value holders (stub/); the fields come from the
 * shared input rows (ProbeSimple.ROWS) through {@link #f}, so the Go side
 * (packets_oracle_character_test.go) mirrors each case by name.
 */
final class ProbeCharacters
{
	// Field i of row v: the rows have ten ints, the index wraps.
	static int f(int[] v, int i)
	{
		return v[i % 10];
	}

	// Float field k of the shared doubles, as the float the status getters return.
	static float fl(int r, int k)
	{
		return (float) ProbeSimple.DOUBLES[(r + k) % ProbeSimple.DOUBLES.length];
	}

	static double dbl(int r, int k)
	{
		return ProbeSimple.DOUBLES[(r + k) % ProbeSimple.DOUBLES.length];
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			summons(r, v, n);
			npcs(r, v, n);
			players(r, v, n);
		}
		variants();
	}

	private static final int[] CLASS_INDEX =
	{
		0,
		18,
		53,
		88
	};

	// The Go twin of the live character: its calculators answer these for the template and
	// resources packets_oracle_character_test.go builds (oracleLive*), pinned there.
	private static void live(Player p)
	{
		final var st = p.playerStatus;
		st.strength = 40;
		st.dexterity = 30;
		st.constitution = 43;
		st.intelligence = 21;
		st.wit = 11;
		st.mentality = 25;
		st.pAtk = 6;
		st.pDef = 38;
		st.mAtk = 3;
		st.mDef = 24;
		st.accuracy = 72;
		st.evasionRate = 72;
		st.criticalHit = 44;
		st.pAtkSpd = 330;
		st.mAtkSpd = 213;
		st.baseRunSpeed = 120;
		st.baseWalkSpeed = 80;
		st.baseSwimSpeed = 50;
		st.movementSpeedMultiplier = 1.1f;
		st.attackSpeedMultiplier = 1.21f;
		st.maxHp = 1227;
		st.maxMp = 126;
		st.maxCp = 86;
	}

	// A player of row r with the shared input; the derived stats are the values the Go
	// calculators answer for a character with no template (all attributes 1, level 40).
	static Player player(int r, int[] v)
	{
		final Player p = new Player();
		p.boatInfo = new BoatInfo();
		p.objectId = f(v, 1);
		p.name = ProbeSimple.STRS[r];
		p.title = ProbeSimple.STRS[(r + 1) % ProbeSimple.STRS.length];
		p.x = f(v, 3);
		p.y = f(v, 4);
		p.z = f(v, 5);
		p.heading = f(v, 6);
		p.race = ClassRace.values()[r];
		p.appearance.sex = Sex.values()[f(v, 2) & 1];
		p.classId = ClassId.values()[CLASS_INDEX[r]];
		p.clanId = f(v, 2);
		p.clanCrestId = f(v, 3);
		p.allyId = f(v, 4);
		p.allyCrestId = f(v, 5);
		p.clanCrestLargeId = f(v, 6);
		p.clanPrivileges = f(v, 7);
		p.pledgeClass = f(v, 8);
		p.pledgeType = f(v, 9);
		p.karma = f(v, 5);
		p.pkKills = f(v, 6);
		p.pvpKills = f(v, 7);
		p.recomLeft = (f(v, 5) & 0x7fffffff) % 10;
		p.recomHave = (f(v, 6) & 0x7fffffff) % 256;
		p.appearance.face = f(v, 6);
		p.appearance.hairStyle = f(v, 7);
		p.appearance.hairColor = f(v, 8);
		p.appearance.nameColor = f(v, 9);
		p.appearance.titleColor = f(v, 0) ^ f(v, 1);
		p.collisionRadius = (f(v, 2) & 1) == 0 ? dbl(r, 2) : dbl(r, 4);
		p.collisionHeight = (f(v, 2) & 1) == 0 ? dbl(r, 3) : dbl(r, 5);

		final var st = p.playerStatus;
		st.level = 40;
		st.exp = (long) f(v, 2) * 1000003L + f(v, 3);
		st.sp = f(v, 8);
		st.hp = f(v, 6) + 0.5;
		st.mp = f(v, 7) + 0.25;
		st.cp = f(v, 8) + 0.75;
		st.maxHp = f(v, 7);
		st.maxMp = f(v, 9);
		st.maxCp = f(v, 0);
		st.strength = 1;
		st.dexterity = 1;
		st.constitution = 1;
		st.intelligence = 1;
		st.wit = 1;
		st.mentality = 1;
		st.pDef = 1;
		st.mDef = 1;
		st.accuracy = 46;
		st.evasionRate = 40;
		st.criticalHit = 34;
		st.pAtkSpd = 255;
		st.mAtkSpd = 133;
		st.movementSpeedMultiplier = 1f;
		st.attackSpeedMultiplier = 0.935f;
		st.inventoryLimit = 80;

		// The equipped items: slot s carries the same object and item ids the character list does.
		for (int slot = 1; slot <= 16; slot++)
		{
			p.inventory.objectIds[slot] = ProbeEngine.paperdollObjectId(v, slot);
			p.inventory.itemIds[slot] = ProbeEngine.paperdollItemId(v, slot);
		}
		p.inventory.augmentationIds[Paperdoll.RHAND.getId()] = f(v, 8);
		p.inventory.augmentationIds[Paperdoll.LHAND.getId()] = f(v, 9);
		if (p.inventory.objectIds[Paperdoll.RHAND.getId()] != 0)
			p.activeWeaponItem = new Object();
		p.enchantEffect = Math.min(127, (f(v, 9) & 0x7fffffff) % 256);
		return p;
	}

	private static void players(int r, int[] v, String n) throws Exception
	{
		final Player p = player(r, v);
		PacketProbe.emit("UserInfo" + n, new UserInfo(p));
		PacketProbe.emit("CharInfo" + n, new CharInfo(p), new GameClient());
	}

	private static final class Variant
	{
		final String name;
		final Consumer<Player> apply;

		Variant(String name, Consumer<Player> apply)
		{
			this.name = name;
			this.apply = apply;
		}
	}

	// Row "a" under one change at a time: every other field is the base row's, so a variant
	// shows the byte or bytes its change moves. The Go twin is oracleVariants.
	private static void variants() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Variant[] all =
		{
			new Variant("live", ProbeCharacters::live),
			new Variant("live-flying", p ->
			{
				live(p);
				p.flying = true;
			}),
			new Variant("gm", p -> p.gm = true),
			new Variant("gm-invisible", p ->
			{
				p.gm = true;
				p.appearance.visible = false;
			}),
			new Variant("invisible", p -> p.appearance.visible = false),
			new Variant("gm-hero-aura", p ->
			{
				p.gm = true;
				Config.GM_HERO_AURA = true;
			}),
			new Variant("hero", p -> p.hero = true),
			new Variant("noble", p -> p.noble = true),
			new Variant("strider", p ->
			{
				p.mounted = true;
				p.mountType = 1;
				p.mountNpcId = 12526;
			}),
			new Variant("wyvern", p ->
			{
				p.mounted = true;
				p.mountType = 2;
				p.mountNpcId = 12621;
				p.flying = true;
			}),
			new Variant("fishing", p ->
			{
				p.fishing = true;
				p.fishingStance.loc = new Location(f(v, 3), f(v, 4), f(v, 5));
			}),
			new Variant("spawn-protected", p ->
			{
				Config.PLAYER_SPAWN_PROTECTION = 1;
				p.spawnProtected = true;
			}),
			new Variant("team-red", p -> p.team = TeamType.RED),
			new Variant("cubics", p ->
			{
				p.cubics.add(new Cubic(1));
				p.cubics.add(new Cubic(3));
			}),
			new Variant("party-room", p -> p.inPartyMatchRoom = true),
			new Variant("abnormal", p -> p.abnormalEffect = 0x2040),
			new Variant("operate-sell", p -> p.operateType = OperateType.PACKAGE_SELL),
			new Variant("crystallize", p -> p.crystallize = true),
			new Variant("clan-leader", p -> p.clanLeader = true),
			new Variant("siege-attacker", p -> p.siegeState = 1),
			new Variant("siege-defender", p -> p.siegeState = 2),
			new Variant("cursed", p ->
			{
				p.cursedWeaponEquippedId = 8190;
				CursedWeaponManager.getInstance().stages.put(8190, 2);
			}),
			new Variant("pvp-flagged", p -> p.pvpFlag = 1),
			new Variant("pvp-blinking", p -> p.pvpFlag = 2),
			new Variant("running", p -> p.running = true),
			new Variant("sitting", p -> p.sitting = true),
			new Variant("combat", p -> p.inCombat = true),
			new Variant("dead", p ->
			{
				p.alikeDead = true;
				p.playerStatus.hp = 0;
			}),
			new Variant("enchant-capped", p -> p.enchantEffect = 127),
			new Variant("boat", p ->
			{
				final BoatInfo info = new BoatInfo();
				info.boat = new Boat();
				info.boat.objectId = f(v, 4);
				p.boatInfo = info;
			}),
		};
		for (final Variant variant : all)
		{
			Config.GM_HERO_AURA = false;
			Config.PLAYER_SPAWN_PROTECTION = 0;
			CursedWeaponManager.getInstance().stages.clear();
			final Player p = player(0, v);
			variant.apply.accept(p);
			PacketProbe.emit("UserInfo/" + variant.name, new UserInfo(p));
			PacketProbe.emit("CharInfo/" + variant.name, new CharInfo(p), new GameClient());
		}
		Config.GM_HERO_AURA = false;
		Config.PLAYER_SPAWN_PROTECTION = 0;
		CursedWeaponManager.getInstance().stages.clear();

		// An invisible player is drawn to a viewer by who the viewer is.
		final Player hidden = player(0, v);
		hidden.appearance.visible = false;
		final GameClient gmViewer = new GameClient();
		gmViewer.player = new Player();
		gmViewer.player.gm = true;
		final GameClient plainViewer = new GameClient();
		plainViewer.player = new Player();
		PacketProbe.emit("CharInfo/invisible-gm-viewer", new CharInfo(hidden), gmViewer);
		PacketProbe.emit("CharInfo/invisible-plain-viewer", new CharInfo(hidden), plainViewer);
	}

	private static void summonBase(Summon s, int r, int[] v)
	{
		s.objectId = f(v, 1);
		s.template.idTemplate = f(v, 2);
		s.x = f(v, 3);
		s.y = f(v, 4);
		s.z = f(v, 5);
		s.heading = f(v, 6);
		s.summonType = f(v, 0);
		s.getStatus().mAtkSpd = f(v, 7);
		s.getStatus().pAtkSpd = f(v, 8);
		s.getStatus().baseRunSpeed = f(v, 9);
		s.getStatus().baseWalkSpeed = f(v, 0) ^ f(v, 5);
		s.getStatus().movementSpeedMultiplier = fl(r, 0);
		s.getStatus().attackSpeedMultiplier = fl(r, 1);
		s.collisionRadius = dbl(r, 2);
		s.collisionHeight = dbl(r, 3);
		s.running = (f(v, 3) & 1) != 0;
		s.inCombat = (f(v, 1) & 1) != 0;
		s.alikeDead = (f(v, 2) & 1) != 0;
		s.name = ProbeSimple.STRS[r];
		s.title = ProbeSimple.STRS[(r + 1) % ProbeSimple.STRS.length];
		s.pvpFlag = (byte) (Math.abs(f(v, 4) % 3));
		s.karma = f(v, 5);
		s.getStatus().hp = f(v, 6);
		s.getStatus().maxHp = f(v, 7);
		s.getStatus().mp = f(v, 8);
		s.getStatus().maxMp = f(v, 9);
		s.getStatus().sp = f(v, 0);
		s.getStatus().level = f(v, 1);
		s.getStatus().exp = (long) f(v, 2) * 1000003L + f(v, 3);
		s.getStatus().expForThisLevel = (long) f(v, 4) * 1000003L + f(v, 5);
		s.getStatus().expForNextLevel = (long) f(v, 6) * 1000003L + f(v, 7);
		s.weightLimit = f(v, 8);
		s.getStatus().pAtk = f(v, 9);
		s.getStatus().pDef = f(v, 0) ^ f(v, 1);
		s.getStatus().mAtk = f(v, 2) ^ f(v, 3);
		s.getStatus().mDef = f(v, 4) ^ f(v, 5);
		s.getStatus().accuracy = f(v, 6) ^ f(v, 7);
		s.getStatus().evasionRate = f(v, 8) ^ f(v, 9);
		s.getStatus().criticalHit = f(v, 0) ^ f(v, 9);
		s.getStatus().moveSpeed = f(v, 3) & 0x7fff;
		s.abnormalEffect = f(v, 5);
		s.mountable = (f(v, 6) & 1) != 0;
		s.move.moveType = MoveType.values()[r % 3];
		s.team = TeamType.values()[r % 3];
		s.soulShotsPerHit = f(v, 7);
		s.spiritShotsPerHit = f(v, 8);
		s.attackable = (f(v, 9) & 1) != 0;
		s.showSummonAnimation = true;
		s.owner = new Player();
		s.owner.name = "Master";
		s.owner.online = true;
	}

	private static void summons(int r, int[] v, String n) throws Exception
	{
		final Pet pet = new Pet();
		summonBase(pet, r, v);
		pet.currentFed = f(v, 4);
		pet.maxMeal = f(v, 5);
		pet.totalWeight = f(v, 6);
		PacketProbe.emit("PetInfo/pet" + n, new PetInfo(pet, 0));
		PacketProbe.emit("PetStatusUpdate/pet" + n, new PetStatusUpdate(pet));
		// A pet shows the weapon and chest armor it wears.
		pet.weapon = f(v, 7) ^ f(v, 1);
		pet.armor = f(v, 8) ^ f(v, 2);
		PacketProbe.emit("PetInfo/pet-worn" + n, new PetInfo(pet, 0));
		PacketProbe.emit("SummonInfo/pet" + n, new AbstractNpcInfo.SummonInfo(pet, null, 0));
		pet.owner.online = false;
		PacketProbe.emit("SummonInfo/pet-offline" + n, new AbstractNpcInfo.SummonInfo(pet, null, 0));
		pet.owner.online = true;
		pet.showSummonAnimation = false;
		PacketProbe.emit("SummonInfo/pet-val" + n, new AbstractNpcInfo.SummonInfo(pet, null, f(v, 3) & 3));

		final Servitor servitor = new Servitor();
		summonBase(servitor, r, v);
		servitor.timeRemaining = f(v, 4);
		servitor.totalLifeTime = f(v, 5);
		PacketProbe.emit("PetInfo/servitor" + n, new PetInfo(servitor, 0));
		PacketProbe.emit("PetStatusUpdate/servitor" + n, new PetStatusUpdate(servitor));
		PacketProbe.emit("SummonInfo/servitor" + n, new AbstractNpcInfo.SummonInfo(servitor, null, 0));
	}

	private static void npcs(int r, int[] v, String n) throws Exception
	{
		final Npc npc = new Npc();
		npc.objectId = f(v, 1);
		npc.template.idTemplate = f(v, 2);
		npc.attackable = (f(v, 9) & 1) != 0;
		npc.x = f(v, 3);
		npc.y = f(v, 4);
		npc.z = f(v, 5);
		npc.heading = f(v, 6);
		npc.getStatus().mAtkSpd = f(v, 7);
		npc.getStatus().pAtkSpd = f(v, 8);
		npc.getStatus().baseRunSpeed = f(v, 9);
		npc.getStatus().baseWalkSpeed = f(v, 0) ^ f(v, 5);
		npc.getStatus().movementSpeedMultiplier = fl(r, 0);
		npc.getStatus().attackSpeedMultiplier = fl(r, 1);
		npc.collisionRadius = dbl(r, 2);
		npc.collisionHeight = dbl(r, 3);
		npc.rightHandItemId = f(v, 4) ^ f(v, 5);
		npc.leftHandItemId = f(v, 6) ^ f(v, 7);
		npc.running = (f(v, 3) & 1) != 0;
		npc.inCombat = (f(v, 1) & 1) != 0;
		npc.alikeDead = (f(v, 2) & 1) != 0;
		npc.template.serverSideName = true;
		npc.template.serverSideTitle = true;
		npc.name = ProbeSimple.STRS[r];
		npc.title = ProbeSimple.STRS[(r + 1) % ProbeSimple.STRS.length];
		npc.abnormalEffect = f(v, 5);
		npc.enchantEffect = f(v, 6);
		npc.flying = (f(v, 7) & 1) != 0;
		npc.move.moveType = MoveType.values()[r % 3];
		PacketProbe.emit("NpcInfo" + n, new AbstractNpcInfo.NpcInfo(npc, null));

		// The stationary view: the hit points show to a viewer who can attack it.
		npc.getStatus().hp = f(v, 8);
		npc.getStatus().maxHp = f(v, 9);
		npc.attackable = true;
		PacketProbe.emit("ServerObjectInfo/attackable" + n, new ServerObjectInfo(npc, null));
		npc.attackable = false;
		PacketProbe.emit("ServerObjectInfo/peaceful" + n, new ServerObjectInfo(npc, null));
	}
}
