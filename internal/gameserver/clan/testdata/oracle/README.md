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
  doubling are crossed. Ascending insertion is how boot restores the stored rows.
- `ops <+offset|-offset, ...> => <iteration order>`: offsets from `0x10000000`. The first adds
  ascend (the boot restore); later `+` are wars declared at runtime (random order, so a war
  goes to the tail of its bucket) and `-` are wars ended (the table never shrinks). Strides of
  16 to 4096 between the ids crowd buckets: past eight ids a bucket makes a table of fewer than
  64 buckets grow early, and from 64 buckets it becomes a tree bucket (newest id first). The
  probe stops removing once a tree bucket exists (read through reflection, hence
  `--add-opens` in `run.sh`): a tree bucket that has shrunk turns back into a chain depending
  on the shape of its tree, which `warSet` does not model (tracked in #4362).
- `sub <types, ascending> => <iteration order of the registry>`: every subset of the seven
  sub-unit types.

## Regenerating

```sh
ACIS_JAVA=/path/to/aCis_gameserver/java internal/gameserver/clan/testdata/oracle/run.sh
```

Needs docker with `eclipse-temurin:21-jdk-alpine`.
