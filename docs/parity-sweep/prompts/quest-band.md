Quest parity review for acis_golang #258, slice {ID} — {title}. This review is read-only: change no code, write no tests, open no PRs.

Pinned SHA: {sha} (wave epic #{wave_epic}). Create your worktree with `git -C acis_golang worktree add ../.worktrees/p258-{ID} {sha}`, so WT=.worktrees/p258-{ID}. Follow docs/parity-sweep/README.md "Repo rules that apply to every run" and "Quest flow".

1. Take this band's review set and its no-scenario list from the P68 issue (#{p68_issue}). If any quest in the set changed after P68's SHA (`git -C $WT log --oneline <P68 sha>..{sha} -- internal/gameserver/script/quest/qNNN`), review its current code.
2. For every quest in the review set, apply README "Parity slice method" steps 1–7 against the Java class quest.Q###_*.
3. Put the no-scenario quests in your oracle-gap list. Record the rest of the band as covered by tooling: a CI fingerprint and page match, with no hand review.

Output: post exactly one comment on #{slice_issue}, in the README's report format, with a "covered by tooling" line. Do not file issues. Remove your worktree when done.
