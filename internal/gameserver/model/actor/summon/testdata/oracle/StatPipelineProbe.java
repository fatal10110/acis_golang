import java.util.ArrayList;
import java.util.Arrays;
import java.util.EnumMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.TreeMap;
import java.util.function.Supplier;

import net.sf.l2j.gameserver.enums.Paperdoll;
import net.sf.l2j.gameserver.enums.items.CrystalType;
import net.sf.l2j.gameserver.enums.items.ItemType;
import net.sf.l2j.gameserver.enums.items.WeaponType;
import net.sf.l2j.gameserver.enums.skills.Stats;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.template.CreatureTemplate;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.item.kind.Item;
import net.sf.l2j.gameserver.skills.basefuncs.Func;
import net.sf.l2j.gameserver.skills.basefuncs.FuncAdd;
import net.sf.l2j.gameserver.skills.basefuncs.FuncAddMul;
import net.sf.l2j.gameserver.skills.basefuncs.FuncBaseAdd;
import net.sf.l2j.gameserver.skills.basefuncs.FuncBaseMul;
import net.sf.l2j.gameserver.skills.basefuncs.FuncEnchant;
import net.sf.l2j.gameserver.skills.basefuncs.FuncMul;
import net.sf.l2j.gameserver.skills.basefuncs.FuncSet;
import net.sf.l2j.gameserver.skills.basefuncs.FuncSub;
import net.sf.l2j.gameserver.skills.basefuncs.FuncSubDiv;

/**
 * Drives the reference stat calculator (compiled by run.sh from the reference
 * sources) through the same cases as the golden scenarios of the summon, npc
 * and player packages and prints the stat each case reads as the JSON object
 * of that package's testdata/golden_stats.json: the case name and the bits of
 * the raw, untruncated double (Double.doubleToLongBits). The mode argument
 * picks the actor kind: summon, npc or player.
 */
public final class StatPipelineProbe
{
	// Cases every actor kind shares: the stat and bases they read.
	private static final class Bases
	{
		final Stats order30;
		final double pAtk;
		final double pDef;
		final double mDef;
		
		Bases(Stats order30, double pAtk, double pDef, double mDef)
		{
			this.order30 = order30;
			this.pAtk = pAtk;
			this.pDef = pDef;
			this.mDef = mDef;
		}
		
		double of(Stats stat)
		{
			return switch (stat)
			{
				case POWER_ATTACK -> pAtk;
				case POWER_DEFENCE -> pDef;
				default -> mDef;
			};
		}
	}
	
	private static double calc(Creature c, Stats stat, double base)
	{
		return c.getStatus().calcStat(stat, base, null, null);
	}
	
	// The move speed getter of a player: a float base through the stat, cut
	// back to float. No swamp, weight or armor grade malus applies here.
	private static double runSpeed(Creature c, float base)
	{
		return (float) c.getStatus().calcStat(Stats.RUN_SPEED, base, null, null);
	}
	
	private static void shared(Map<String, Double> out, Supplier<Creature> actor, Bases b)
	{
		// Three order-30 funcs attached one call at a time keep their attach
		// order; 1e16 swallows the unit in one order and not the other.
		{
			final Creature a = actor.get();
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, b.order30, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncSub(null, b.order30, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, b.order30, 1, null)));
			out.put("order30_forward", calc(a, b.order30, b.of(b.order30)));
		}
		{
			final Creature a = actor.get();
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, b.order30, 1, null)));
			a.addStatFuncs(List.<Func> of(new FuncSub(null, b.order30, 1e16, null)));
			a.addStatFuncs(List.<Func> of(new FuncAdd(null, b.order30, 1e16, null)));
			out.put("order30_reverse", calc(a, b.order30, b.of(b.order30)));
		}
		
		// A Set rebases the base a later BaseMul reads; both run before the
		// builtin (order 10).
		{
			final Creature a = actor.get();
			a.addStatFuncs(Arrays.<Func> asList(
				new FuncSet(null, Stats.MAGIC_DEFENCE, 500, null),
				new FuncBaseMul(null, Stats.MAGIC_DEFENCE, 0.5, null)));
			out.put("set_rebase_mdef", calc(a, Stats.MAGIC_DEFENCE, b.mDef));
		}
		
		// Funcs attached for an owner act while it is attached and leave the
		// builtin chain as it was once the owner's funcs are removed.
		{
			final Creature a = actor.get();
			final Object owner = new Object();
			final double before = calc(a, Stats.POWER_ATTACK, b.pAtk);
			a.addStatFuncs(Arrays.<Func> asList(
				new FuncAdd(owner, Stats.POWER_ATTACK, 7, null),
				new FuncMul(owner, Stats.POWER_ATTACK, 1.25, null)));
			final double during = calc(a, Stats.POWER_ATTACK, b.pAtk);
			a.removeStatsByOwner(owner);
			final double after = calc(a, Stats.POWER_ATTACK, b.pAtk);
			out.put("attach_detach_before", before);
			out.put("attach_detach_during", during);
			out.put("attach_detach_after", after);
		}
	}
	
	private static Map<String, Double> summon()
	{
		// The summon test's CombatStats: STR 40, MEN 20, P.Atk 100, P.Def 50,
		// M.Def 40, at level 44.
		final Map<String, Double> out = new TreeMap<>();
		shared(out, () -> new Creature(new CreatureTemplate(40, 0, 0, 0, 0, 20), 44),
			new Bases(Stats.POWER_DEFENCE, 100, 50, 40));
		return out;
	}
	
	private static Map<String, Double> npc()
	{
		// The npc test's template: a level-20 monster, STR 40, CON 21, DEX 30,
		// INT 20, WIT 43, MEN 20, P.Atk 100, P.Def 50, M.Def 40.
		final Map<String, Double> out = new TreeMap<>();
		shared(out, () -> new Creature(new CreatureTemplate(40, 21, 30, 20, 43, 20), 20),
			new Bases(Stats.POWER_DEFENCE, 100, 50, 40));
		return out;
	}
	
	// The player test's template: STR 40, CON 43, DEX 30, INT 21, WIT 11, MEN
	// 25, P.Atk 5, P.Def 50, M.Def 60, M.Atk 20 and run speed 100 where a
	// case reads them.
	private static final double PLAYER_P_ATK = 5;
	private static final double PLAYER_P_DEF = 50;
	private static final double PLAYER_M_DEF = 60;
	private static final double PLAYER_M_ATK = 20;
	private static final float PLAYER_RUN_SPEED = 100;
	
	private static Player player(int level, boolean mage)
	{
		return new Player(new CreatureTemplate(40, 43, 30, 21, 11, 25), level, mage);
	}
	
	private static final int SLOT_RHAND = 0x0080;
	private static final int SLOT_CHEST = 0x0400;
	
	// A worn piece by the token the Go scenarios use for it.
	private static void wear(Player p, String token)
	{
		final Paperdoll slot;
		int bodyPart = 0;
		switch (token)
		{
			case "head" -> slot = Paperdoll.HEAD;
			case "chest" ->
			{
				slot = Paperdoll.CHEST;
				bodyPart = SLOT_CHEST;
			}
			case "fullbody" ->
			{
				slot = Paperdoll.CHEST;
				bodyPart = Item.SLOT_FULL_ARMOR;
			}
			case "alldress" ->
			{
				slot = Paperdoll.CHEST;
				bodyPart = Item.SLOT_ALLDRESS;
			}
			case "legs" -> slot = Paperdoll.LEGS;
			case "gloves" -> slot = Paperdoll.GLOVES;
			case "feet" -> slot = Paperdoll.FEET;
			case "lfinger" -> slot = Paperdoll.LFINGER;
			case "rfinger" -> slot = Paperdoll.RFINGER;
			case "lear" -> slot = Paperdoll.LEAR;
			case "rear" -> slot = Paperdoll.REAR;
			case "neck" -> slot = Paperdoll.NECK;
			default -> throw new IllegalArgumentException(token);
		}
		p.getInventory().equip(slot, new ItemInstance(new Item(bodyPart, CrystalType.NONE, null), 0));
	}
	
	// penalty adds the P.Def and M.Def of a level-40 player wearing the pieces.
	private static void penalty(Map<String, Double> out, String name, boolean mage, String pieces)
	{
		final Player p = player(40, mage);
		for (String token : pieces.split(","))
			wear(p, token);
		out.put("penalty_" + name + "_pdef", calc(p, Stats.POWER_DEFENCE, PLAYER_P_DEF));
		out.put("penalty_" + name + "_mdef", calc(p, Stats.MAGIC_DEFENCE, PLAYER_M_DEF));
	}
	
	private static final String ARMOR = "head,chest,legs,gloves,feet";
	private static final String JEWELRY = "lfinger,rfinger,lear,rear,neck";
	
	private static Player enchantOwner(Func f, Player p)
	{
		p.addStatFuncs(List.<Func> of(f));
		return p;
	}
	
	private static final WeaponType[] ENCHANT_TYPES = { WeaponType.SWORD, WeaponType.BOW, WeaponType.BIGSWORD, WeaponType.BIGBLUNT, WeaponType.DUAL, WeaponType.DUALFIST };
	
	private static ItemInstance enchanted(int bodyPart, CrystalType crystal, ItemType type, int enchant)
	{
		return new ItemInstance(new Item(bodyPart, crystal, type), enchant);
	}
	
	private static Map<String, Double> player()
	{
		final Map<String, Double> out = new TreeMap<>();
		shared(out, () -> player(1, false), new Bases(Stats.POWER_ATTACK, PLAYER_P_ATK, PLAYER_P_DEF, PLAYER_M_DEF));
		
		// Several stats at once, each also carrying its builtin at order 10.
		{
			final Player p = player(1, false);
			p.addStatFuncs(Arrays.<Func> asList(
				new FuncBaseAdd(null, Stats.POWER_DEFENCE, 4, null),
				new FuncMul(null, Stats.POWER_DEFENCE, 1.1, null),
				new FuncAdd(null, Stats.MAGIC_ATTACK, 6, null),
				new FuncAddMul(null, Stats.MAGIC_ATTACK, 10, null),
				new FuncSubDiv(null, Stats.RUN_SPEED, 20, null)));
			out.put("mixed_pdef", calc(p, Stats.POWER_DEFENCE, PLAYER_P_DEF));
			out.put("mixed_matk", calc(p, Stats.MAGIC_ATTACK, PLAYER_M_ATK));
			out.put("mixed_runspeed", runSpeed(p, PLAYER_RUN_SPEED));
		}
		
		// The builtins alone, at a level where the level mod is not below 1.
		{
			final Player p = player(40, false);
			out.put("builtin_patk", calc(p, Stats.POWER_ATTACK, PLAYER_P_ATK));
			out.put("builtin_matk", calc(p, Stats.MAGIC_ATTACK, PLAYER_M_ATK));
			out.put("builtin_pdef", calc(p, Stats.POWER_DEFENCE, PLAYER_P_DEF));
			out.put("builtin_mdef", calc(p, Stats.MAGIC_DEFENCE, PLAYER_M_DEF));
			out.put("builtin_runspeed", runSpeed(p, PLAYER_RUN_SPEED));
		}
		
		// Armor and jewelry penalties of the defence builtins, fighter and
		// mage class, with a chest piece, a full-body suit or a one-piece
		// outfit.
		penalty(out, "head", false, "head");
		penalty(out, "chest_fighter", false, "chest");
		penalty(out, "chest_mage", true, "chest");
		penalty(out, "chest_legs_mage", true, "chest,legs");
		penalty(out, "fullbody_fighter", false, "fullbody");
		penalty(out, "fullbody_mage", true, "fullbody");
		penalty(out, "alldress_fighter", false, "alldress");
		penalty(out, "legs_fighter", false, "legs");
		penalty(out, "legs_mage", true, "legs");
		penalty(out, "gloves_feet", false, "gloves,feet");
		penalty(out, "armor_fighter", false, ARMOR);
		penalty(out, "armor_mage", true, ARMOR);
		penalty(out, "fingers", false, "lfinger,rfinger");
		penalty(out, "ears", false, "lear,rear");
		penalty(out, "neck", false, "neck");
		penalty(out, "jewelry", false, JEWELRY);
		penalty(out, "all_fighter", false, ARMOR + "," + JEWELRY);
		penalty(out, "all_mage", true, ARMOR + "," + JEWELRY);
		
		// Enchant bonuses by crystal grade, weapon type and level.
		for (CrystalType grade : CrystalType.values())
		{
			final String g = grade.name().toLowerCase(Locale.ROOT);
			for (int enchant : new int[]
			{
				2,
				7
			})
			{
				for (WeaponType type : ENCHANT_TYPES)
				{
					final Player p = enchantOwner(new FuncEnchant(enchanted(SLOT_RHAND, grade, type, enchant), Stats.POWER_ATTACK, 0, null), player(1, false));
					out.put("enchant_patk_" + g + "_" + type.name().toLowerCase(Locale.ROOT) + "_e" + enchant, calc(p, Stats.POWER_ATTACK, PLAYER_P_ATK));
				}
				
				final Player m = enchantOwner(new FuncEnchant(enchanted(SLOT_RHAND, grade, WeaponType.SWORD, enchant), Stats.MAGIC_ATTACK, 0, null), player(1, false));
				out.put("enchant_matk_" + g + "_e" + enchant, calc(m, Stats.MAGIC_ATTACK, PLAYER_M_ATK));
			}
		}
		// The boundary levels of the first three enchant steps.
		for (int enchant : new int[]
		{
			0,
			3,
			4
		})
		{
			final Player p = enchantOwner(new FuncEnchant(enchanted(SLOT_RHAND, CrystalType.S, WeaponType.SWORD, enchant), Stats.POWER_ATTACK, 0, null), player(1, false));
			out.put("enchant_patk_s_sword_e" + enchant, calc(p, Stats.POWER_ATTACK, PLAYER_P_ATK));
		}
		// Armor and jewelry add a flat amount per step, whatever the grade.
		for (int enchant : new int[]
		{
			2,
			7
		})
		{
			final Player pd = enchantOwner(new FuncEnchant(enchanted(SLOT_CHEST, CrystalType.A, null, enchant), Stats.POWER_DEFENCE, 0, null), player(1, false));
			out.put("enchant_pdef_e" + enchant, calc(pd, Stats.POWER_DEFENCE, PLAYER_P_DEF));
			final Player md = enchantOwner(new FuncEnchant(enchanted(SLOT_CHEST, CrystalType.A, null, enchant), Stats.MAGIC_DEFENCE, 0, null), player(1, false));
			out.put("enchant_mdef_e" + enchant, calc(md, Stats.MAGIC_DEFENCE, PLAYER_M_DEF));
			// A non-weapon item leaves P.Atk alone.
			final Player pa = enchantOwner(new FuncEnchant(enchanted(SLOT_CHEST, CrystalType.A, null, enchant), Stats.POWER_ATTACK, 0, null), player(1, false));
			out.put("enchant_patk_armor_e" + enchant, calc(pa, Stats.POWER_ATTACK, PLAYER_P_ATK));
		}
		return out;
	}
	
	public static void main(String[] args)
	{
		Locale.setDefault(Locale.ROOT);
		final Map<String, Double> out = switch (args[0])
		{
			case "summon" -> summon();
			case "npc" -> npc();
			case "player" -> player();
			default -> throw new IllegalArgumentException(args[0]);
		};
		
		final List<String> lines = new ArrayList<>();
		for (Map.Entry<String, Double> e : out.entrySet())
			lines.add("  \"" + e.getKey() + "\": " + Double.doubleToLongBits(e.getValue()));
		System.out.print("{\n" + String.join(",\n", lines) + "\n}");
	}
}
