import net.sf.l2j.gameserver.enums.GaugeColor;
import net.sf.l2j.gameserver.enums.StatusType;
import net.sf.l2j.gameserver.enums.skills.FlyType;
import net.sf.l2j.gameserver.model.actor.Creature;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.location.Location;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * Packets that read a world object or creature. The creature is a stand-in
 * value holder built from the same input rows the Go side replays
 * (ProbeSimple.ROWS; packets_oracle_cases_test.go): object id, x, y, z, heading
 * in v[0..4].
 */
final class ProbeWorld
{
	private static Creature creature(int objectId, int x, int y, int z, int heading)
	{
		final Creature c = new Creature();
		c.objectId = objectId;
		c.x = x;
		c.y = y;
		c.z = z;
		c.heading = heading;
		return c;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final int[] w = ProbeSimple.ROWS[(r + 1) % ProbeSimple.ROWS.length];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			final Creature c = creature(v[0], v[1], v[2], v[3], v[4]);

			// MoveToLocation reads the destination from the creature's move, or takes it.
			c.move = new BoatMove();
			c.move.destination = new Location(v[5], v[6], v[7]);
			PacketProbe.emit("MoveToLocation" + n, new MoveToLocation(c));
			PacketProbe.emit("MoveToLocation/explicit" + n, new MoveToLocation(c, new Location(v[8], v[9], v[5])));
			PacketProbe.emit("MoveToPawn" + n, new MoveToPawn(c, creature(v[5], 0, 0, 0, 0), v[6]));
			PacketProbe.emit("ValidateLocation" + n, new ValidateLocation(c));
			PacketProbe.emit("TeleportToLocation/slow" + n, new TeleportToLocation(c, v[5], v[6], v[7], false));
			PacketProbe.emit("TeleportToLocation/fast" + n, new TeleportToLocation(c, v[5], v[6], v[7], true));
			PacketProbe.emit("SocialAction" + n, new SocialAction(c, v[5]));
			PacketProbe.emit("DeleteObject" + n, new DeleteObject(c));
			PacketProbe.emit("DeleteObject/seated" + n, new DeleteObject(c, true));
			PacketProbe.emit("DeleteObject/standing" + n, new DeleteObject(c, false));
			PacketProbe.emit("TargetUnselected" + n, new TargetUnselected(c));
			PacketProbe.emit("Revive" + n, new Revive(c));
			PacketProbe.emit("StopMove" + n, new StopMove(c));
			PacketProbe.emit("Earthquake/npc" + n, new Earthquake(c, v[5], v[6], true));
			PacketProbe.emit("Earthquake/player" + n, new Earthquake(c, v[5], v[6], false));
			PacketProbe.emit("Earthquake/default" + n, new Earthquake(c, v[5], v[6]));

			for (final int wait : new int[] { 0, 1, 2, 3, v[5] })
				PacketProbe.emit("ChangeWaitType/" + wait + n, new ChangeWaitType(c, wait));

			for (final boolean running : new boolean[] { true, false })
				for (final boolean swimming : new boolean[] { true, false })
				{
					c.running = running;
					c.inWater = swimming;
					PacketProbe.emit("ChangeMoveType/" + running + "-" + swimming + n, new ChangeMoveType(c));
				}

			final Creature target = creature(w[0], w[1], w[2], w[3], w[4]);
			PacketProbe.emit("MagicSkillUse/hit" + n, new MagicSkillUse(c, target, v[5], v[6], v[7], v[8], false));
			PacketProbe.emit("MagicSkillUse/crit" + n, new MagicSkillUse(c, target, v[5], v[6], v[7], v[8], true));
			PacketProbe.emit("MagicSkillUse/self" + n, new MagicSkillUse(c, v[5], v[6], v[7], v[8]));

			final StatusUpdate status = new StatusUpdate(c);
			status.addAttribute(StatusType.LEVEL, v[5]);
			status.addAttribute(StatusType.CUR_HP, v[6]);
			status.addAttribute(StatusType.KARMA, v[7]);
			status.addAttribute(StatusType.MAX_CP, v[8]);
			PacketProbe.emit("StatusUpdate" + n, status);
			PacketProbe.emit("StatusUpdate/empty" + n, new StatusUpdate(c));

			for (final FlyType type : FlyType.values())
				PacketProbe.emit("FlyToLocation/" + type.ordinal() + n, new FlyToLocation(c, v[5], v[6], v[7], type));

			for (final GaugeColor color : GaugeColor.values())
			{
				PacketProbe.emit("SetupGauge/" + color.ordinal() + n, new SetupGauge(color, v[0], v[1]));
				PacketProbe.emit("SetupGauge/single-" + color.ordinal() + n, new SetupGauge(color, v[0]));
			}
		}

		for (int i = 0; i < ProbeSimple.STRS.length; i++)
		{
			final Creature titled = creature(0x01020304, 0, 0, 0, 0);
			titled.title = ProbeSimple.STRS[i];
			PacketProbe.emit("TitleUpdate/" + ProbeSimple.STR_NAMES[i], new TitleUpdate(titled));
		}
	}
}
