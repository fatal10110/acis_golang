import net.sf.l2j.gameserver.model.actor.Npc;
import net.sf.l2j.gameserver.model.actor.Player;
import net.sf.l2j.gameserver.model.actor.move.BoatMove;
import net.sf.l2j.gameserver.model.actor.template.NpcTemplate;
import net.sf.l2j.gameserver.network.serverpackets.*;

/**
 * The polymorphed views: PcMorphInfo, the NpcInfo other players get for a polymorphed
 * player; the UserInfo a polymorphed player gets; and the NpcInfo of a polymorphed npc.
 * The player and the npc are ProbeCharacters' of the same row, so the Go side
 * (packets_oracle_polymorph_test.go) mirrors each case by name.
 */
final class ProbePolymorph
{
	// The npc form of row r: its npc id, template id, name, held items and body all differ
	// from the row's own values.
	static NpcTemplate form(int r, int[] v)
	{
		final NpcTemplate t = new NpcTemplate();
		t.npcId = ProbeCharacters.f(v, 7);
		t.idTemplate = ProbeCharacters.f(v, 8);
		t.name = ProbeSimple.STRS[(r + 2) % ProbeSimple.STRS.length];
		t.rightHand = ProbeCharacters.f(v, 9) ^ ProbeCharacters.f(v, 0);
		t.leftHand = ProbeCharacters.f(v, 1) ^ ProbeCharacters.f(v, 2);
		t.collisionRadius = ProbeCharacters.dbl(r, 4);
		t.collisionHeight = ProbeCharacters.dbl(r, 5);
		return t;
	}

	static Player morphed(int r, int[] v)
	{
		final Player p = ProbeCharacters.player(r, v);
		p.move = new BoatMove();
		p.polymorphTemplate = form(r, v);
		return p;
	}

	static void run() throws Exception
	{
		for (int r = 0; r < ProbeSimple.ROWS.length; r++)
		{
			final int[] v = ProbeSimple.ROWS[r];
			final String n = "/" + ProbeSimple.ROW_NAMES[r];
			final Player p = morphed(r, v);
			PacketProbe.emit("PcMorphInfo" + n, new AbstractNpcInfo.PcMorphInfo(p, p.polymorphTemplate));
			PacketProbe.emit("UserInfo/polymorph" + n, new UserInfo(p));

			final Npc npc = ProbeCharacters.npc(r, v);
			npc.polymorphTemplate = form(r, v);
			PacketProbe.emit("NpcInfo/polymorph" + n, new AbstractNpcInfo.NpcInfo(npc, null));
		}

		// Row "a" with the live speeds, running, in combat, dead and under a visual effect.
		final int[] a = ProbeSimple.ROWS[0];
		final Player p = morphed(0, a);
		ProbeCharacters.live(p);
		p.running = true;
		p.inCombat = true;
		p.alikeDead = true;
		p.abnormalEffect = 0x2040;
		PacketProbe.emit("PcMorphInfo/live", new AbstractNpcInfo.PcMorphInfo(p, p.polymorphTemplate));
	}
}
