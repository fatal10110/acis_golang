# Final sigma oracle

`FinalSigmaProbe.java` captures how the reference server lower-cases client
text (`String.toLowerCase()` on JDK 21, en_US) where a capital sigma is
involved, and the tables that decision depends on.

Regenerate from this directory (the outputs land beside the probe; move them
into `testdata/` and `wordbreak_tables.go`):

```sh
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD":/w -w /w eclipse-temurin:21-jdk-alpine sh -c '
  javac -d /tmp FinalSigmaProbe.java &&
  { java -cp /tmp FinalSigmaProbe read < hand.in;
    java -cp /tmp FinalSigmaProbe gen 1 1500 8;
    java -cp /tmp FinalSigmaProbe gen 2 200 30; } > final_sigma.golden &&
  java -cp /tmp --add-opens java.base/java.lang=ALL-UNNAMED FinalSigmaProbe cased > cased.golden &&
  java -cp /tmp --add-opens java.base/sun.text=ALL-UNNAMED FinalSigmaProbe tables > tables.txt'
```

- `final_sigma.golden`: one row per input, `<input units>\t<lower-case units>`
  in hex. The first rows are the hand-picked inputs of `hand.in`; the rest are
  random strings over the probe's alphabet, every one holding a capital sigma.
  `TestUTF16LowerKeyFinalSigma` replays them.
- `cased.golden`: the code point ranges `isCased` accepts.
  `TestIsCasedMatchesOracle` compares every code point.
- `tables.txt` (not kept): the word iterator's forward and backward state
  tables (`stateTable`, `backwardsStateTable`, 22 columns), accepting states
  (`endStates`; no lookahead states), and the category of every code point as
  `FIRST-LAST:category` ranges. `wordbreak_tables.go` holds them verbatim: the
  state tables and accepting states as printed, and each range as its first
  code point shifted left by 8 with the category plus one in the low byte.

The same run, widened to 790,000 random rows (`gen` with other seeds and
lengths up to 30), matched `UTF16LowerKey` row for row when the port was made.
