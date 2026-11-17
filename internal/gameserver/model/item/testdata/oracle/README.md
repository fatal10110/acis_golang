# Drop merge and sweep order oracle

`../order.golden` is the expected output of `TestDropOrderMatchesReferenceProbe`
(`../../oracle_order_test.go`): the order the reference hands a kill's drops and
a sweep's items out in, written by `OrderProbe.java`. `DropCategory` and
`SpoilState` are the reference classes compiled unmodified; `stub/` replaces
the drop rate, the player and monster types and the id/value holder, which
reach the config and the skill table. Every row rolls with chance 100 and a
fixed quantity, so nothing is random.

Lines (rows are `<item id>x<quantity>`, comma separated; `-` is an empty map):

- `merge <name> <rate> <rows> <result>`: one spoil category rolled `rate`
  times, the order of the map it returns.
- `sweep <name> <categories> <result>`: the categories (`;` separated) rolled
  in turn into one spoil state, each result put into it, then the order a sweep
  iterates it in. An item two categories roll keeps the amount the later one
  puts and the place the first one took.

Regenerate with `run.sh` (set `ACIS_JAVA` when the reference is not next to the
Go repository).
