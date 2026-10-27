# Login/link crypt oracle

`../crypt.golden` is the expected output of `TestCryptMatchesReferenceProbe`
(`../../oracle_test.go`).

`run.sh` compiles the reference classes **unmodified** from aCis 409
`aCis_gameserver/java/net/sf/l2j/`:

- `loginserver/crypt/BlowfishEngine.java`
- `loginserver/crypt/NewCrypt.java` (checksum and link encryption)
- `loginserver/crypt/ScrambledKeyPair.java` (`scrambleModulus`, called
  through reflection because it is private)
- `gameserver/network/gameserverpackets/GameServerBasePacket.java` (the link
  packet's checksum slot and 8-byte padding)

and runs `CryptProbe.java` against them. The probe's `link` lines follow the
game server's link send path (`LoginServerThread.sendPacket`: `getContent()`,
`NewCrypt.appendChecksum`, `NewCrypt.crypt`) with the bootstrap key
`"_;v.]05-31!|+-%xT!^[$\00"` or an explicit session key. Inputs are the Go
tests' earlier vectors plus a deterministic xorshift filler.

Line formats:

- `checksum <in> <verifyChecksum(in)> <appendChecksum(in)|-> <verifyChecksum(out)>`
- `blowfish <key> <plaintext> <ciphertext>` (each 8-byte block through `processBlock`)
- `link <session key|- for the bootstrap key> <payload> <padded with checksum> <encrypted>`
- `scramble <modulus as BigInteger.toByteArray()> <scrambled modulus>`

## Regenerating

Needs docker. From the repository root, with `ACIS_JAVA` pointing at the
reference source root (default: `aCis_gameserver/java` next to the Go
repository):

```
ACIS_JAVA=/path/to/aCis_gameserver/java internal/commons/crypt/testdata/oracle/run.sh
```
