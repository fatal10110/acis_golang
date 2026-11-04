import java.util.LinkedHashMap;
import java.util.Map;
import java.util.function.Consumer;

import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.item.instance.ItemInstance;
import net.sf.l2j.gameserver.model.pledge.Clan;
import net.sf.l2j.gameserver.model.pledge.ClanMember;
import net.sf.l2j.gameserver.network.serverpackets.ExGetBossRecord;
import net.sf.l2j.gameserver.network.serverpackets.GMViewCharacterInfo;
import net.sf.l2j.gameserver.network.serverpackets.GMViewQuestList;
import net.sf.l2j.gameserver.network.serverpackets.GMViewSkillInfo;
import net.sf.l2j.gameserver.network.serverpackets.GMViewWarehouseWithdrawList;
import net.sf.l2j.gameserver.scripting.Quest;
import net.sf.l2j.gameserver.scripting.QuestState;
import net.sf.l2j.gameserver.skills.L2Skill;

/**
 * The game master's views of a character (status, skills, quests, warehouse) over the player,
 * skill, quest and item stand-ins of the other families, and the raid point record.
 */
final class ProbeGMView
{
	static int f(int[] v, int i)
	{
		return v[i % v.length];
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			PacketProbe.emit("GMViewCharacterInfo" + n, new GMViewCharacterInfo(ProbeCharacters.player(r, v)));
			PacketProbe.emit("GMViewSkillInfo" + n, new GMViewSkillInfo(skills(r, v, false, null)));
			PacketProbe.emit("GMViewQuestList" + n, new GMViewQuestList(quests(r, v)));
			warehouses(r, v, n);
			bossRecord(v, n);
		}
		characterVariants();
		skillFlags();

		final Player nobody = new Player();
		nobody.name = "Bo";
		PacketProbe.emit("GMViewSkillInfo/empty", new GMViewSkillInfo(nobody));
		PacketProbe.emit("GMViewQuestList/empty", new GMViewQuestList(nobody));
		PacketProbe.emit("GMViewWarehouseWithdrawList/empty", new GMViewWarehouseWithdrawList(nobody));
		PacketProbe.emit("ExGetBossRecord/none", new ExGetBossRecord(3, 7, null));
		PacketProbe.emit("ExGetBossRecord/empty", new ExGetBossRecord(0, 0, new LinkedHashMap<>()));
	}

	private static void characterVariants() throws Exception
	{
		final int[] v = ProbeSimple.ROWS[0];
		final Map<String, Consumer<Player>> all = new LinkedHashMap<>();
		all.put("live", ProbeCharacters::live);
		all.put("live-flying", p ->
		{
			ProbeCharacters.live(p);
			p.flying = true;
		});
		all.put("gm", p -> p.gm = true);
		all.put("hero", p -> p.hero = true);
		all.put("noble", p -> p.noble = true);
		for (Map.Entry<String, Consumer<Player>> e : all.entrySet())
		{
			final Player p = ProbeCharacters.player(0, v);
			e.getValue().accept(p);
			PacketProbe.emit("GMViewCharacterInfo/" + e.getKey(), new GMViewCharacterInfo(p));
		}
		// The current gauges cast to int: NaN, infinities and values outside the int range.
		final double[] d = ProbeParty.VITALS;
		for (int i = 0; i < d.length; i++)
		{
			final Player p = ProbeCharacters.player(0, v);
			p.playerStatus.hp = d[i];
			p.playerStatus.mp = d[(i + 1) % d.length];
			p.playerStatus.cp = d[(i + 2) % d.length];
			PacketProbe.emit("GMViewCharacterInfo/vitals" + i, new GMViewCharacterInfo(p));
		}
	}

	// Four skills: the odd ones passive, the last two clan skills (ProbeSkills.skillListPlayer).
	private static Player skills(int r, int[] v, boolean formalWear, Clan clan)
	{
		final Player p = new Player();
		p.name = ProbeSimple.STRS[r];
		for (int k = 0; k < 4; k++)
		{
			final L2Skill s = ProbeSkills.skill(f(v, k), f(v, k + 1), false);
			s.passive = k % 2 == 1;
			s.clanSkill = k >= 2;
			p.skills.put(k, s);
		}
		p.wearingFormalWear = formalWear;
		p.clan = clan;
		return p;
	}

	// Formal wear greys every skill out; a clan skill greys out under a negative reputation.
	private static void skillFlags() throws Exception
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
				PacketProbe.emit("GMViewSkillInfo/formal" + formal + "-clan-" + clans[c], new GMViewSkillInfo(skills(0, v, formal == 1, clan)));
			}
		}
	}

	private static Player quests(int r, int[] v)
	{
		final Player p = new Player();
		p.name = ProbeSimple.STRS[(r + 1) % ProbeSimple.STRS.length];
		for (int k = 0; k < 3; k++)
		{
			final QuestState qs = new QuestState();
			qs.quest = new Quest(f(v, k));
			qs.flags = f(v, k + 1);
			p.questList.states.add(qs);
		}
		return p;
	}

	// The shared five-item list (a worn augmented weapon, a worn shadow armor, an augmented jewel, a
	// quest stack and adena) and an unaugmented weapon, in a player's and in a clan's warehouse.
	private static void warehouses(int r, int[] v, String n) throws Exception
	{
		final Player p = new Player();
		p.name = ProbeSimple.STRS[r];
		p.warehouse.adena = f(v, 2);
		for (ItemInstance it : ProbeItems.items(v))
			p.warehouse.items.add(it);
		p.warehouse.items.add(ProbeItems.item(v, ProbeItems.WEAPON, false, false));
		PacketProbe.emit("GMViewWarehouseWithdrawList/player" + n, new GMViewWarehouseWithdrawList(p));

		final Clan clan = new Clan();
		final Player leader = new Player();
		leader.name = ProbeSimple.STRS[(r + 2) % ProbeSimple.STRS.length];
		clan.leader = new ClanMember(clan, leader);
		clan.warehouse.adena = f(v, 3);
		for (ItemInstance it : ProbeItems.items(ProbeSimple.ROWS[(r + 1) % ProbeSimple.ROWS.length]))
			clan.warehouse.items.add(it);
		PacketProbe.emit("GMViewWarehouseWithdrawList/clan" + n, new GMViewWarehouseWithdrawList(clan));
	}

	// Three bosses (distinct ids in every row) in the order the record lists them.
	private static void bossRecord(int[] v, String n) throws Exception
	{
		final Map<Integer, Integer> points = new LinkedHashMap<>();
		for (int k = 0; k < 3; k++)
			points.put(f(v, k) + 7 * k, f(v, k + 3));
		PacketProbe.emit("ExGetBossRecord" + n, new ExGetBossRecord(f(v, 6), f(v, 7), points));
	}
}
