Quest triage for acis_golang #258, slice {ID} — {title}. This review is read-only: change no code, write no tests, open no PRs.

Pinned SHA: {sha} (wave epic #{wave_epic}). Create your worktree with `git -C acis_golang worktree add ../.worktrees/p258-{ID} {sha}`, so WT=.worktrees/p258-{ID}. Follow docs/parity-sweep/README.md "Repo rules that apply to every run" and "Quest flow".

CI runs the per-quest fingerprint and page tests, so at a green SHA they pass. Your job is to rank the quests so the bands review the riskiest ones.

1. Collect the facts for every quest package under $WT/internal/gameserver/script/quest/ (341 quests; secondclass and thirdclass are shared code):
   - scenario: does tests/quest exercise it? (rg for the package name and quest id under $WT/tests/quest);
   - rewards: does it give adena (item 57) or other items?
   - repeatable: can the quest be repeated?
   - gated: does it check class, race or level beyond a plain minimum?
   - tests: did its fingerprint, page or scenario test fail or skip in the CI run or the local evidence on #{wave_epic}?
2. Compute each band's review set (bands P69–P79, from docs/parity-sweep/manifest.json):
   a. every quest whose tests failed or skipped;
   b. every quest with rewards and no scenario;
   c. then the highest-ranked remaining quests, until the set holds max(5, 15% of the band's quests, rounded up). Rank by the number of signals among rewards, repeatable, gated and no-scenario, then by quest id.
3. Post one comment on #{slice_issue}:
   - the run summary;
   - per band: the review set, each quest with its signals;
   - per band: the list of quests with no scenario, which goes to that band's oracle-gap issue.
Do not file issues. Remove your worktree when done.
