import java.util.HexFormat;

import net.sf.l2j.gameserver.network.GameCrypt;

// Drives the unmodified reference game-client cipher (compiled from the
// reference tree by run.sh, against stub/net/sf/l2j/Config.java) through
// fixed packet sequences and prints every transform in order:
//   key <16-byte key>   starts a fresh cipher instance
//   enc <in> <out>      encrypt(raw, 0, raw.length)
//   dec <in> <out>      decrypt(raw, 0, raw.length)
// The first encrypt on an instance only arms it, as on the wire.
public class CipherProbe
{
	private static final HexFormat HEX = HexFormat.of();
	private static int _seed = 0x1B873593;
	
	private static byte[] fill(int n)
	{
		final byte[] b = new byte[n];
		for (int i = 0; i < n; i++)
		{
			_seed ^= _seed << 13;
			_seed ^= _seed >>> 17;
			_seed ^= _seed << 5;
			b[i] = (byte) _seed;
		}
		return b;
	}
	
	private static GameCrypt start(byte[] key)
	{
		final GameCrypt c = new GameCrypt();
		c.setKey(key);
		System.out.println("key " + HEX.formatHex(key));
		return c;
	}
	
	private static void enc(GameCrypt c, byte[] raw)
	{
		final String in = HEX.formatHex(raw);
		c.encrypt(raw, 0, raw.length);
		System.out.println("enc " + in + " " + HEX.formatHex(raw));
	}
	
	private static void dec(GameCrypt c, byte[] raw)
	{
		final String in = HEX.formatHex(raw);
		c.decrypt(raw, 0, raw.length);
		System.out.println("dec " + in + " " + HEX.formatHex(raw));
	}
	
	public static void main(String[] args)
	{
		final byte[] fixedKey = HEX.parseHex("101112131415161718191a1b1c1d1e1f");
		final int[] sizes = { 1, 2, 3, 4, 7, 8, 9, 15, 16, 17, 64, 255, 256, 1024 };
		
		// Encrypt sequence with the Go test's fixed key and packets.
		GameCrypt c = start(fixedKey.clone());
		enc(c, new byte[16]);
		enc(c, HEX.parseHex("48656c6c6f2c20776f726c6421212121"));
		enc(c, HEX.parseHex("7365636f6e64207061636b657420626f647921"));
		enc(c, HEX.parseHex("33726421"));
		for (int n : sizes)
			enc(c, fill(n));
		
		// Decrypt before arming is a no-op; arm with the first encrypt, then decrypt.
		for (int k = 0; k < 3; k++)
		{
			final byte[] key = (k == 0) ? fixedKey.clone() : fill(16);
			c = start(key);
			dec(c, fill(8));
			enc(c, fill(16));
			for (int n : sizes)
			{
				dec(c, fill(n));
				enc(c, fill(n));
			}
		}
	}
}
