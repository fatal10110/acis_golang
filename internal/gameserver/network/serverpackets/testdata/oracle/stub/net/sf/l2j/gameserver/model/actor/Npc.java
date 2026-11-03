package net.sf.l2j.gameserver.model.actor;

import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.status.CreatureStatus;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.model.residence.castle.Castle;

// Stand-in for a world npc: a plain value holder for the getters the probed
// packets read.
public class Npc extends Creature
{
	public NpcTemplate template = new NpcTemplate();
	public NpcTemplate polymorphTemplate;
	public CreatureStatus npcStatus = new CreatureStatus();
	public int rightHandItemId;
	public int leftHandItemId;
	public int enchantEffect;
	public boolean attackable;
	public Castle castle;
	public int clanId;

	public Npc()
	{
		move = new BoatMove();
	}

	public final NpcTemplate getTemplate()
	{
		return template;
	}

	public final NpcTemplate getPolymorphTemplate()
	{
		return polymorphTemplate;
	}

	@Override
	public CreatureStatus getStatus()
	{
		return npcStatus;
	}

	public int getRightHandItemId()
	{
		return rightHandItemId;
	}

	public int getLeftHandItemId()
	{
		return leftHandItemId;
	}

	public int getEnchantEffect()
	{
		return enchantEffect;
	}

	public boolean isAttackableWithoutForceBy(Player attacker)
	{
		return attackable;
	}

	public boolean isAttackableBy(Creature attacker)
	{
		return attackable;
	}

	public final Castle getCastle()
	{
		return castle;
	}

	public int getClanId()
	{
		return clanId;
	}
}
