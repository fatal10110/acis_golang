package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.enums.TeamType;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.CreatureStatus;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;

// Stand-in for a summon: a plain value holder for the getters the probed
// packets read.
public class Summon extends Creature
{
	public Player owner;
	public NpcTemplate template = new NpcTemplate();
	public CreatureStatus summonStatus = new CreatureStatus();
	public int summonType;
	public byte pvpFlag;
	public int karma;
	public boolean mountable;
	public int soulShotsPerHit;
	public int spiritShotsPerHit;
	public int weightLimit;
	public TeamType team = TeamType.NONE;
	public boolean attackable;

	public Summon()
	{
		move = new BoatMove();
	}

	public final Player getOwner()
	{
		return owner;
	}

	public NpcTemplate getTemplate()
	{
		return template;
	}

	@Override
	public CreatureStatus getStatus()
	{
		return summonStatus;
	}

	public int getSummonType()
	{
		return summonType;
	}

	// The base summon wears nothing; a pet overrides both.
	public int getWeapon()
	{
		return 0;
	}

	public int getArmor()
	{
		return 0;
	}

	public final byte getPvpFlag()
	{
		return pvpFlag;
	}

	public int getKarma()
	{
		return karma;
	}

	public boolean isMountable()
	{
		return mountable;
	}

	public int getSoulShotsPerHit()
	{
		return soulShotsPerHit;
	}

	public int getSpiritShotsPerHit()
	{
		return spiritShotsPerHit;
	}

	public int getWeightLimit()
	{
		return weightLimit;
	}

	public final TeamType getTeam()
	{
		return team;
	}

	public boolean isAttackableWithoutForceBy(Player attacker)
	{
		return attackable;
	}
}
