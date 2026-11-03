import java.util.ArrayList;
import java.util.List;

import net.sf.l2j.commons.data.StatSet;
import net.sf.l2j.gameserver.data.xml.RecipeData;
import net.sf.l2j.gameserver.enums.ShortcutType;
import net.sf.l2j.gameserver.enums.actors.ClassId;
import net.sf.l2j.gameserver.enums.skills.AcquireSkillType;
import net.sf.l2j.gameserver.model.Augmentation;
import net.sf.l2j.gameserver.model.Shortcut;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.attack.CreatureAttack.HitHolder;
import net.sf.l2j.gameserver.model.actor.instance.Door;
import net.sf.l2j.gameserver.model.actor.instance.StaticObject;
import net.sf.l2j.gameserver.model.holder.IntIntHolder;
import net.sf.l2j.gameserver.model.holder.skillnode.ClanSkillNode;
import net.sf.l2j.gameserver.model.holder.skillnode.EnchantSkillNode;
import net.sf.l2j.gameserver.model.holder.skillnode.FishingSkillNode;
import net.sf.l2j.gameserver.model.holder.skillnode.GeneralSkillNode;
import net.sf.l2j.gameserver.model.holder.skillnode.SkillNode;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.EtcItem;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.records.Recipe;
import net.sf.l2j.gameserver.model.records.Timestamp;
import net.sf.l2j.gameserver.model.residence.castle.Castle;
import net.sf.l2j.gameserver.network.serverpackets.*;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.QuestState;
import net.sf.l2j.gameserver.skills.L2Skill;

/**
 * Skills and effects (effect lists, skill list, reuse timers, shortcut bar, skill trainers),
 * attacks, doors, the quest list and the gauge windows of the olympiad and the duel. Every
 * field is a function of the shared row v through {@link ProbeCharacters#f}, so the Go side
 * (packets_oracle_skills_test.go) builds the same values by name. What the stand-ins decide
 * before the encoder (which reuse timers are still running, the greyed-out flag of a skill, an
 * item shortcut's reuse and augmentation) is handed to each Go case as the values the reference
 * reads.
 */
final class ProbeSkills
{
	static int f(int[] v, int i)
	{
		return ProbeCharacters.f(v, i);
	}

	static L2Skill skill(int id, int level, boolean toggle)
	{
		final L2Skill s = new L2Skill();
		s.id = id;
		s.level = level;
		s.toggle = toggle;
		return s;
	}

	// Durations in ms: whole seconds truncate toward zero, -1 is the permanent marker.
	static final int[] DURATIONS =
	{
		999,
		1000,
		1001,
		15999,
		-1,
		-999,
		-1000,
		-1999,
		Integer.MIN_VALUE,
		Integer.MAX_VALUE
	};

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			effects(r, v, n);
			skillList(r, v, n);
			reuse(r, v, n);
			launched(r, v, n);
			trainers(r, v, n);
			shortcuts(r, v, n);
			attack(r, v, n);
			doors(r, v, n);
			quests(r, v, n);
			gauges(r, v, n);
		}
		effectOrders();
		skillListFlags();
		reuseUnits();
		attackFlags();
		doorResidences();
		vitals();

		PacketProbe.emit("AcquireSkillDone", AcquireSkillDone.STATIC_PACKET);
	}

	// ---- effect lists ----

	private static void effects(int r, int[] v, String n) throws Exception
	{
		// Skills 2 and 5 are toggles.
		final AbnormalStatusUpdate list = new AbnormalStatusUpdate();
		for (int k = 0; k < 6; k++)
			list.addEffect(skill(f(v, k), f(v, k + 1), k % 3 == 2), f(v, k + 2));
		PacketProbe.emit("AbnormalStatusUpdate" + n, list);

		// The same list as the olympiad spectators see it: the fourth entry is a toggle, which
		// gets no special encoding there.
		final ExOlympiadSpelledInfo spelled = new ExOlympiadSpelledInfo(withId(f(v, 0)));
		for (int k = 0; k < 4; k++)
			spelled.addEffect(skill(f(v, k), f(v, k + 1), k == 3), f(v, k + 2));
		PacketProbe.emit("ExOlympiadSpelledInfo" + n, spelled);
		PacketProbe.emit("ExOlympiadSpelledInfo/empty" + n, new ExOlympiadSpelledInfo(withId(f(v, 1))));
		if (r == 0)
		{
			PacketProbe.emit("AbnormalStatusUpdate/empty", new AbnormalStatusUpdate());
			final AbnormalStatusUpdate timed = new AbnormalStatusUpdate();
			final ExOlympiadSpelledInfo timedSpelled = new ExOlympiadSpelledInfo(withId(f(v, 0)));
			for (int ms : DURATIONS)
			{
				timed.addEffect(skill(1000 + ms % 100, 3, false), ms);
				timedSpelled.addEffect(skill(1000 + ms % 100, 3, false), ms);
			}
			// A toggle never reads its duration.
			timed.addEffect(skill(7, 1, true), 5000);
			PacketProbe.emit("AbnormalStatusUpdate/durations", timed);
			PacketProbe.emit("ExOlympiadSpelledInfo/durations", timedSpelled);
		}
	}

	private static Player withId(int objectId)
	{
		final Player p = new Player();
		p.objectId = objectId;
		return p;
	}

	// The toggles come after the other effects, sorted by id, and a second toggle of an id is dropped.
	private static void effectOrders() throws Exception
	{
		final AbnormalStatusUpdate list = new AbnormalStatusUpdate();
		list.addEffect(skill(30, 1, true), 1000);
		list.addEffect(skill(7, 2, false), -1);
		list.addEffect(skill(10, 3, true), 2000);
		list.addEffect(skill(8, 4, false), 5000);
		list.addEffect(skill(20, 5, true), 3000);
		list.addEffect(skill(10, 6, true), 4000);
		list.addEffect(skill(-5, 7, true), 5000);
		list.addEffect(skill(30, 8, true), 6000);
		list.addEffect(skill(7, 9, false), 7000);
		PacketProbe.emit("AbnormalStatusUpdate/orders", list);
	}

	// ---- skill list ----

	private static void skillList(int r, int[] v, String n) throws Exception
	{
		PacketProbe.emit("SkillList" + n, new SkillList(skillListPlayer(v, false, null)));
		if (r == 0)
			PacketProbe.emit("SkillList/empty", new SkillList(new Player()));
	}

	// Four skills: the odd ones passive, the last two clan skills.
	private static Player skillListPlayer(int[] v, boolean formalWear, Clan clan)
	{
		final Player p = new Player();
		for (int k = 0; k < 4; k++)
		{
			final L2Skill s = skill(f(v, k), f(v, k + 1), false);
			s.passive = k % 2 == 1;
			s.clanSkill = k >= 2;
			p.skills.put(k, s);
		}
		p.wearingFormalWear = formalWear;
		p.clan = clan;
		return p;
	}

	// Formal wear greys every skill out; a clan skill greys out under a negative reputation.
	private static void skillListFlags() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final String[] clans =
		{
			"none",
			"neg",
			"zero",
			"pos"
		};
		final int[] reputation =
		{
			0,
			-1,
			0,
			5
		};
		for (int formal = 0; formal < 2; formal++)
		{
			for (int c = 0; c < clans.length; c++)
			{
				Clan clan = null;
				if (c > 0)
				{
					clan = new Clan();
					clan.reputation = reputation[c];
				}
				PacketProbe.emit("SkillList/formal" + formal + "-clan-" + clans[c], new SkillList(skillListPlayer(v, formal == 1, clan)));
			}
		}
	}

	// ---- reuse timers ----

	private static void reuse(int r, int[] v, String n) throws Exception
	{
		// Four timers, reuse f(v,k+2) ms, f(v,k+3) ms left of it; one that is not positive has run out.
		final Player p = new Player();
		for (int k = 0; k < 4; k++)
			p.reuseTimeStamps.add(new Timestamp(skill(f(v, k), f(v, k + 1), false), f(v, k + 2), Timestamp.NOW + f(v, k + 3)));
		PacketProbe.emit("SkillCoolTime" + n, new SkillCoolTime(p));
		if (r == 0)
			PacketProbe.emit("SkillCoolTime/empty", new SkillCoolTime(new Player()));
	}

	private static void reuseUnits() throws Exception
	{
		final Player p = new Player();
		int id = 1;
		for (int reuse : new int[] { 999, 1000, 1001, 15999, -1, -999, -1999, 600000, Integer.MAX_VALUE })
		{
			for (int left : new int[] { 1, 999, 1000, 1001, 600000, Integer.MAX_VALUE })
				p.reuseTimeStamps.add(new Timestamp(skill(id++, 1, false), reuse, Timestamp.NOW + left));
		}
		PacketProbe.emit("SkillCoolTime/units", new SkillCoolTime(p));
	}

	// ---- cast launch ----

	private static void launched(int r, int[] v, String n) throws Exception
	{
		final L2Skill s = skill(f(v, 1), f(v, 2), false);
		for (int count : new int[] { 0, 1, 3 })
		{
			final Creature[] targets = new Creature[count];
			for (int i = 0; i < count; i++)
			{
				targets[i] = new Creature();
				targets[i].objectId = f(v, 3 + i);
			}
			final Creature caster = new Creature();
			caster.objectId = f(v, 0);
			PacketProbe.emit("MagicSkillLaunched/t" + count + n, new MagicSkillLaunched(caster, s, targets));
		}
	}

	// ---- skill trainers ----

	private static StatSet node(int[] v, int k)
	{
		return new StatSet().set("id", f(v, k)).set("lvl", f(v, k + 1)).set("minLvl", f(v, k + 2)).set("cost", f(v, k + 3)).set("itemId", f(v, k + 4)).set("itemCount", f(v, k + 5));
	}

	private static void trainers(int r, int[] v, String n) throws Exception
	{
		final List<EnchantSkillNode> enchant = new ArrayList<>();
		final List<GeneralSkillNode> usual = new ArrayList<>();
		final List<FishingSkillNode> fishing = new ArrayList<>();
		final List<ClanSkillNode> clan = new ArrayList<>();
		for (int k = 0; k < 3; k++)
		{
			enchant.add(new EnchantSkillNode(node(v, k).set("exp", f(v, k + 6)).set("sp", f(v, k + 7)).set("rate76", 0).set("rate77", 0).set("rate78", 0).set("rate79", 0).set("rate80", 0)));
			usual.add(new GeneralSkillNode(node(v, k)));
			fishing.add(new FishingSkillNode(node(v, k)));
			clan.add(new ClanSkillNode(node(v, k)));
		}
		PacketProbe.emit("ExEnchantSkillList" + n, new ExEnchantSkillList(enchant));
		PacketProbe.emit("AcquireSkillList/usual" + n, new AcquireSkillList(AcquireSkillType.USUAL, usual));
		PacketProbe.emit("AcquireSkillList/fishing" + n, new AcquireSkillList(AcquireSkillType.FISHING, fishing));
		PacketProbe.emit("AcquireSkillList/clan" + n, new AcquireSkillList(AcquireSkillType.CLAN, clan));
		if (r == 0)
		{
			PacketProbe.emit("ExEnchantSkillList/empty", new ExEnchantSkillList(new ArrayList<EnchantSkillNode>()));
			PacketProbe.emit("AcquireSkillList/usual-empty", new AcquireSkillList(AcquireSkillType.USUAL, new ArrayList<SkillNode>()));
		}
	}

	// ---- shortcut bar ----

	private static final String[] SHORTCUTS =
	{
		"item-missing",
		"item-plain",
		"item-augmented",
		"item-etc-no-skill",
		"item-etc-skill",
		"item-etc-timer",
		"skill",
		"action",
		"macro",
		"recipe",
		"none"
	};

	// The object id of the item shortcut k: distinct inside a row, even where the row is all zero.
	static int shortcutId(int[] v, int k)
	{
		return k < 6 ? f(v, k + 2) + k * 0x101 : f(v, k + 2);
	}

	private static ShortcutType shortcutType(int k)
	{
		if (k < 6)
			return ShortcutType.ITEM;
		return new ShortcutType[] { ShortcutType.SKILL, ShortcutType.ACTION, ShortcutType.MACRO, ShortcutType.RECIPE, ShortcutType.NONE }[k - 6];
	}

	private static Shortcut shortcut(int[] v, int k)
	{
		final Shortcut s = new Shortcut(f(v, k), f(v, k + 1), shortcutType(k), shortcutId(v, k), f(v, k + 3), f(v, k + 4));
		s.setSharedReuseGroup(f(v, k + 5));
		return s;
	}

	// The inventory and reuse timers behind the item shortcuts 1 to 5.
	private static Player shortcutOwner(int[] v)
	{
		final Player p = new Player();
		for (int k = 1; k < 6; k++)
		{
			final ItemInstance item = new ItemInstance(shortcutId(v, k), new Item(), 1);
			if (k == 2)
				item.augmentation = new Augmentation(f(v, 7));
			if (k >= 3)
			{
				item.etc = new EtcItem();
				if (k >= 4)
				{
					final L2Skill itemSkill = skill(1, 1, false);
					itemSkill.reuseDelay = f(v, 8);
					itemSkill.reuseHashCode = f(v, 6) + k;
					final IntIntHolder holder = new IntIntHolder(1, 1);
					holder.skill = itemSkill;
					item.etc.skills = new IntIntHolder[] { holder, new IntIntHolder(2, 2) };
					if (k == 5)
						p.reuseTimeStamp.put(itemSkill.reuseHashCode, new Timestamp(itemSkill, f(v, 8), Timestamp.NOW + f(v, 9)));
				}
			}
			p.inventory.byObjectId.put(item.objectId, item);
		}
		return p;
	}

	private static void shortcuts(int r, int[] v, String n) throws Exception
	{
		final Player owner = shortcutOwner(v);
		final Shortcut[] all = new Shortcut[SHORTCUTS.length];
		for (int k = 0; k < all.length; k++)
		{
			all[k] = shortcut(v, k);
			PacketProbe.emit("ShortCutRegister/" + SHORTCUTS[k] + n, new ShortCutRegister(owner, all[k]));
		}
		owner.shortcutList.shortcuts = all;
		PacketProbe.emit("ShortCutInit" + n, new ShortCutInit(owner));
		if (r == 0)
		{
			owner.shortcutList.shortcuts = new Shortcut[0];
			PacketProbe.emit("ShortCutInit/empty", new ShortCutInit(owner));
		}
	}

	// ---- attacks ----

	private static void attack(int r, int[] v, String n) throws Exception
	{
		final Creature attacker = new Creature();
		attacker.objectId = f(v, 0);
		attacker.x = f(v, 1);
		attacker.y = f(v, 2);
		attacker.z = f(v, 3);
		for (int count : new int[] { 1, 2, 5 })
		{
			final HitHolder[] hits = new HitHolder[count];
			for (int i = 0; i < count; i++)
				hits[i] = new HitHolder(f(v, 3 + i), f(v, 4 + i), f(v, 5 + i) & 0xff);
			PacketProbe.emit("Attack/hits" + count + n, new Attack(attacker, hits));
		}
	}

	// Every flag the swing sets, alone and together.
	private static void attackFlags() throws Exception
	{
		final Creature attacker = new Creature();
		attacker.objectId = 1;
		final int[] flags =
		{
			0,
			Attack.HITFLAG_SS,
			Attack.HITFLAG_CRIT,
			Attack.HITFLAG_SHLD,
			Attack.HITFLAG_MISS,
			Attack.HITFLAG_SS | Attack.HITFLAG_CRIT,
			Attack.HITFLAG_SS | Attack.HITFLAG_CRIT | Attack.HITFLAG_SHLD | Attack.HITFLAG_MISS
		};
		final HitHolder[] hits = new HitHolder[flags.length];
		for (int i = 0; i < flags.length; i++)
			hits[i] = new HitHolder(100 + i, 10 * i, flags[i]);
		PacketProbe.emit("Attack/flags", new Attack(attacker, hits));
	}

	// ---- doors, static objects ----

	private static Door door(int[] v, boolean attackable)
	{
		final Door d = new Door();
		d.objectId = f(v, 0);
		d.doorId = f(v, 1);
		d.opened = (f(v, 2) & 1) == 1;
		d.damage = f(v, 5);
		d.attackable = attackable;
		d.doorStatus.maxHp = f(v, 3);
		d.doorStatus.hp = f(v, 4) + 0.5;
		return d;
	}

	private static void doors(int r, int[] v, String n) throws Exception
	{
		PacketProbe.emit("DoorInfo" + n, new DoorInfo(withId(0), door(v, false)));
		PacketProbe.emit("DoorInfo/attackable" + n, new DoorInfo(withId(0), door(v, true)));
		final Door sieged = door(v, false);
		sieged.residence = siegeCastle(true);
		PacketProbe.emit("DoorStatusUpdate" + n, new DoorStatusUpdate(sieged));
		final StaticObject so = new StaticObject();
		so.staticObjectId = f(v, 0);
		so.objectId = f(v, 1);
		PacketProbe.emit("StaticObjectInfo" + n, new StaticObjectInfo(so));
	}

	private static Castle siegeCastle(boolean inProgress)
	{
		final Castle c = new Castle(1);
		c.siege.side = net.sf.l2j.gameserver.enums.SiegeSide.PENDING;
		c.siege.inProgress = inProgress;
		return c;
	}

	// The status packet shows the hit points only for a door of a castle under siege.
	private static void doorResidences() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Door none = door(v, false);
		PacketProbe.emit("DoorStatusUpdate/no-residence", new DoorStatusUpdate(none));
		final Door idle = door(v, false);
		idle.residence = siegeCastle(false);
		PacketProbe.emit("DoorStatusUpdate/castle-idle", new DoorStatusUpdate(idle));
		final Door hall = door(v, false);
		hall.residence = new Object();
		PacketProbe.emit("DoorStatusUpdate/other-residence", new DoorStatusUpdate(hall));
	}

	// ---- quest list ----

	private static void quests(int r, int[] v, String n) throws Exception
	{
		final Player p = new Player();
		for (int k = 0; k < 3; k++)
		{
			final QuestState qs = new QuestState();
			qs.quest = new Quest(f(v, k));
			qs.flags = f(v, k + 1);
			p.questList.states.add(qs);
		}
		PacketProbe.emit("QuestList" + n, new QuestList(p));
		if (r == 0)
			PacketProbe.emit("QuestList/empty", new QuestList(new Player()));
	}

	// ---- olympiad and duel gauges ----

	static Player gauges(int r, int[] v, double hp, double mp, double cp)
	{
		final Player p = new Player();
		p.objectId = f(v, 1);
		p.name = ProbeSimple.STRS[r];
		p.classId = ClassId.values()[ProbeCharacters.CLASS_INDEX[r]];
		p.olympiadSide = f(v, 0);
		final var st = p.playerStatus;
		st.level = f(v, 2);
		st.hp = hp;
		st.mp = mp;
		st.cp = cp;
		st.maxHp = f(v, 3);
		st.maxMp = f(v, 4);
		st.maxCp = f(v, 5);
		return p;
	}

	private static void gauges(int r, int[] v, String n) throws Exception
	{
		final Player p = gauges(r, v, ProbeCharacters.dbl(r, 0), ProbeCharacters.dbl(r, 1), ProbeCharacters.dbl(r, 2));
		PacketProbe.emit("ExOlympiadUserInfo" + n, new ExOlympiadUserInfo(p));
		PacketProbe.emit("ExDuelUpdateUserInfo" + n, new ExDuelUpdateUserInfo(p));
	}

	// Each current gauge in every position: the packets cast the double to an int, and the
	// character, recipe and shop windows read the same status.
	private static void vitals() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final double[] d = ProbeParty.VITALS;
		for (int i = 0; i < d.length; i++)
		{
			final String name = "/vitals" + i;
			final double hp = d[i];
			final double mp = d[(i + 1) % d.length];
			final double cp = d[(i + 2) % d.length];
			final Player p = gauges(0, v, hp, mp, cp);
			PacketProbe.emit("ExOlympiadUserInfo" + name, new ExOlympiadUserInfo(p));
			PacketProbe.emit("ExDuelUpdateUserInfo" + name, new ExDuelUpdateUserInfo(p));

			final Player self = ProbeCharacters.player(0, v);
			self.playerStatus.hp = hp;
			self.playerStatus.mp = mp;
			self.playerStatus.cp = cp;
			PacketProbe.emit("UserInfo" + name, new UserInfo(self));
			PacketProbe.emit("CharInfo" + name, new CharInfo(self), new net.sf.l2j.gameserver.network.GameClient());

			// The crafter's mana in the recipe windows; the maximum is an int there.
			final Player crafter = new Player();
			crafter.objectId = f(v, 0);
			crafter.playerStatus.mp = mp;
			crafter.playerStatus.maxMp = f(v, 3);
			crafter.recipeBook.dwarven.add(new Recipe(f(v, 4), true));
			crafter.recipeBook.dwarven.add(new Recipe(f(v, 5), true));
			crafter.recipeBook.common.add(new Recipe(f(v, 6), false));
			final Player buyer = new Player();
			buyer.adena = f(v, 2);
			PacketProbe.emit("RecipeShopSellList" + name, new RecipeShopSellList(buyer, crafter));
			PacketProbe.emit("RecipeShopItemInfo" + name, new RecipeShopItemInfo(crafter, f(v, 4)));
			PacketProbe.emit("RecipeBookItemList" + name, new RecipeBookItemList(crafter, true));
			RecipeData.getInstance().recipes.clear();
			RecipeData.getInstance().recipes.put(f(v, 4), new Recipe(f(v, 4), true));
			PacketProbe.emit("RecipeItemMakeInfo" + name, new RecipeItemMakeInfo(f(v, 4), crafter));
			RecipeData.getInstance().recipes.clear();
		}
	}
}
