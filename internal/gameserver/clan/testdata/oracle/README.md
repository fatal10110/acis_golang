# Clan registry order oracle

`../order.golden` is the expected output of `TestRegistryOrderMatchesReferenceProbe`
(`../../order_oracle_test.go`).

`run.sh` cuts two field declarations **verbatim** out of
`aCis_gameserver/java/net/sf/l2j/gameserver/model/pledge/Clan.java` (only CRLF line ends are
normalized) and compiles them unchanged inside the harness `ClanCopy.java.tmpl`:

- `_atWarWith = ConcurrentHashMap.newKeySet()` (the war set; the attacker set is declared
  the same way),
- `_subPledges = new ConcurrentHashMap<>()` (the sub-unit registry).

`OrderProbe.java` fills them and prints their iteration order on the real JDK 21 collections.

Line formats:

- `war <ids, ascending> => <iteration order>`: war sets of 1 to 100 clans, with ids spread
  over 64 to 4 million values from `0x10000000`, so tables of 16 to 256 buckets and every
  doubling are crossed. Ascending insertion is how boot restores the stored rows. A later
  declared war inserts at the tail of its bucket in the reference, which the Go side does
  not model (see `hashSetOrder`'s comment); the probe does not cover that, nor buckets of
  more than eight ids (tree bins and early growth); the remaining drifts are tracked in #4341.
- `sub <types, ascending> => <iteration order of the registry>`: every subset of the seven
  sub-unit types.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/clan/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
