# Game client cipher oracle

`../cipher.golden` is the expected output of `TestCipherMatchesReferenceProbe`
(`../../oracle_test.go`).

`run.sh` compiles aCis 409
`aCis_gameserver/java/net/sf/l2j/gameserver/network/GameCrypt.java`
**unmodified**, against `stub/net/sf/l2j/Config.java`, which only provides
`USE_BLOWFISH_CIPHER = true` (the shipped setting and the only configuration
the cipher reads), and runs `CipherProbe.java`.

Each `key` line starts a fresh `GameCrypt` with `setKey`; each `enc`/`dec`
line is one `encrypt`/`decrypt` call over the whole buffer, in order, so the
golden pins the arming on the first encrypt, the no-op decrypt before
arming and the per-packet key roll. The first sequence uses the Go tests'
fixed key `101112131415161718191a1b1c1d1e1f` and packets; the rest come from
a deterministic xorshift filler.

## Regenerating

Needs docker. From the repository root, with `ACIS_JAVA` pointing at the
reference source root (default: `aCis_gameserver/java` next to the Go
repository):

```
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/network/cipher/testdata/oracle/run.sh
```
