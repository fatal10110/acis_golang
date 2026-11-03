package net.sf.l2j.gameserver.model.actor.attack;

// Stand-in for the attack engine: only the hit record the Attack packet lists.
public final class CreatureAttack
{
	public static class HitHolder
	{
		public int _targetId;
		public int _damage;
		public int _flags;

		public HitHolder(int targetId, int damage, int flags)
		{
			_targetId = targetId;
			_damage = damage;
			_flags = flags;
		}
	}
}
