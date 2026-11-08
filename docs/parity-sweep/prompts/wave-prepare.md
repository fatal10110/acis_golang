Prepare wave {N} of the acis_golang #258 parity sweep (epic #{wave_epic}). Phase A of 2: refresh the manifest and open its PR. Do not change Go code. Work from the outer acis_public/ root.

1. Create a worktree at the tip of main:
   `git -C acis_golang fetch origin main && git -C acis_golang worktree add ../.worktrees/p258-prep{N} origin/main`
   Call it WT=.worktrees/p258-prep{N}.
2. Run the slicer: `python3 $WT/docs/parity-sweep/slicer.py $WT $WT/docs/parity-sweep/manifest.json`.
   - If it exits non-zero, fix only `$WT/docs/parity-sweep/slicer.py` and rerun until it exits 0. Add a rule or an override for each unassigned file, and settle each routing tie in HANDLER_OVERRIDES.
3. Review the diff of `$WT/docs/parity-sweep/manifest.json` against main (`git -C $WT diff`):
   - For every file that is new to the manifest, or has moved to another slice, check that its slice is right. Catch-all rules and globs absorb new files silently. Fix wrong placements with an override and rerun step 2.
   - A new slice takes the next free ID (P82, ...). Add it to WAVES and to a domain in slicer.py.
4. If issues.json changed since main (delta issues recorded during the last wave start), copy it into `$WT/docs/parity-sweep/issues.json`.
5. If anything changed:
   a. Commit the changes on the branch `codex/p258-wave{N}-manifest`.
   b. Open a PR titled "parity sweep: manifest for wave {N}" that refs #258.
   c. Post the PR link on #{wave_epic} and ask the owner to merge it.
   If nothing changed, post "manifest unchanged at <origin/main sha>" on #{wave_epic}.
6. Remove the worktree. Phase B (prompts/wave-start.md) runs after the merge.
