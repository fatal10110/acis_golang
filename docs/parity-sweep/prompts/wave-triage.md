Triage wave {N} of the acis_golang #258 parity sweep (epic #{wave_epic}). Do not change Go code. Work from the outer acis_public/ root.

Inputs: the review comment on each slice and delta issue under #{wave_epic}, and the wave's evidence comment.

1. Check that every slice and delta issue in the wave has its review comment. If any is missing, stop and report it on #{wave_epic}.
2. Collect all findings and merge duplicates (same Go line or same Java method). Keep the highest severity and list every slice that reported it.
3. Route findings tagged `owner: P##` to the owning slice, as README "Findings outside your scope" describes.
4. Search before filing, as README "Repo rules" describes:
   a. run issue_search.py with --state all;
   b. run a live check: `gh issue list -R fatal10110/acis_golang --state all --label parity-sweep --search '<terms>'`, because the cache misses issues filed earlier in this sweep;
   c. `gh issue view` each plausible match.
   Link an existing issue instead of filing a duplicate.
5. File follow-ups as README "Triage, follow-ups and closing" describes:
   - one issue per S0–S2 finding;
   - one S3 batch issue per slice that has S3 findings;
   - one oracle-gap issue per slice that has gaps.
   Each body carries the fields README requires. Each goes on milestone M14 with the labels parity-sweep, sev-S# (or oracle-gap) and wave-{N}, and is a sub-issue of its slice or delta issue. Links use the child's numeric id: `gh api -X POST repos/fatal10110/acis_golang/issues/<parent>/sub_issues -F sub_issue_id=<id>`. Space API calls about a second apart.
6. File flaky tests from the evidence under the D20 issue (#{d20_issue}), with the existing flaky-test label.
7. Closing needs the owner's recorded authorization on #258. With it:
   - close each slice and delta issue with a comment listing its follow-ups, or "no drift";
   - close #{wave_epic} once all of them are closed.
   Without it, post the closing list on #{wave_epic}.
8. If {N} is the last wave ({last}):
   a. Launch the final domain runs: `python3 <worktree>/docs/parity-sweep/sweep.py runs --final`, each filled with `sweep.py domain-prompt <run> --sha <wave {N} pinned sha>`.
   b. When they have reported, run the final domain triage: `sweep.py domain-triage {N}`.
