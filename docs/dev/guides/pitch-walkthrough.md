# The pitch's 2.3 to 2.4 story in Claude

The product pitch's worked example is a mandatory acceptance test (PR-27): a callback failure reproduced in production on version 2.3, the fix merged, 2.4 deployed and verified, and an older investigation imported weeks later. This walkthrough tells the story to Claude in plain words and checks that Froodbase answers each question correctly: what is true now, what was true then, and everything with labels. `TestSlice2Exit` in `internal/apitest` makes the same calls through the client `froodbase-mcp` uses, with the records of `domaintest.PitchRecords` (architecture design section 6); keep the two in step.

The service, the work items and the pull request are made up.

## Before you start

1. Follow steps 1 and 2 of the [five-minute walkthrough](../ops/development.md#five-minute-walkthrough-in-claude): the stack is up, the binaries are built and `froodbase-mcp` is registered as bob, the contributor.
2. Start from an empty database if you can (`docker compose down -v`, then `make up`). Otherwise the five-minute walkthrough's records about the import tool's callbacks also come back; leave them out when you compare.

## Steps

1. **The failure, 2 September.** Say: "INC-12: on 2 September 2026 we reproduced a callback failure in production on version 2.3 of the Callback Service. Work item https://dev.azure.com/org/p/_workitems/edit/410, revision 3, has it, and so does the repro runbook https://github.com/org/callbacks/blob/main/runbooks/repro.md at main@2026-09-02." Claude searches with `mode: historical`, finds nothing, and contributes the Callback Service entity, both sources as evidence and an observation with `valid_from` 2026-09-02, no `valid_to`, and scope `environment: prod`, `version_min` and `version_max` 2.3. It reports two roots. Note the observation's id.
2. **The merge, 5 September.** Say: "The fix was merged on 5 September in https://github.com/org/callbacks/pull/418." Claude records an observation with `valid_from` 2026-09-05 and scope `environment: repository`. A pull request link is not a kind Froodbase recognises, so Claude passes `source_kind: repository` (if it forgets, the refusal tells it to). Then say: "What is true about the Callback Service's callbacks in production on 2.3 now?" Claude searches in `current` mode with that scope: the failure is returned, `status: current`; the merge is not. The merge is about the repository, so it does not correct anything in production, and the failure is still active.
3. **The deployment, 9 September.** Say: "On 9 September 2.4 was deployed to production and the callback remediation verified; it's in work item 410, revision 9." Claude records an observation with `valid_from` 2026-09-09 and scope `environment: prod`, versions 2.4.
4. **The supersession.** The deployment supersedes the failure. No tool declares a supersession until slice 4, so set the state as the owner, with the failure's id from step 1:

   ```bash
   docker compose exec -T postgres psql -U froodbase -d froodbase -c "update assertion set lifecycle_state = 'superseded' where id = '<failure id>'"
   ```

   The failure's validity is unchanged: nobody said when it ended.
5. **Ask in each mode.** Say: "What is true about the Callback Service's callbacks now?" Claude searches in `current` mode: the merge and the deployment, both `status: current`. The failure is not returned. Say: "What was true on 2 September?" Claude searches with `mode: as_of` and `as_of: 2026-09-02T00:00:00Z`: the failure, labelled `historical` and `superseded`. Say: "Show everything, past included." Claude searches with `mode: historical`: all three, the failure labelled `historical` and `superseded`, the merge and the deployment `current`.
6. **The late import, today.** Say: "Import the old investigation: work item https://dev.azure.com/org/p/_workitems/edit/77, revision 1, concluded callbacks on 2.3 failed in production from 2 September until the 2.4 deployment on 9 September. It also backs up the INC-12 failure." Claude searches with `mode: historical`, finds the failure, and in one call records the work item as evidence, an inference with `valid_from` 2026-09-02, `valid_to` 2026-09-09 and scope prod, 2.3, and the work item under `links` for the failure. It reports the failure under `linked` with `roots_added: 1` and three roots, still at revision 1.
7. **Ask again.** "What is true now?" gives the same answer as in step 5: neither the failure nor the investigation is current, although the investigation was recorded today. "What was true on 2 September?" now returns the failure and the investigation, both labelled `historical`. "Show the failure" (`get_record`): `lifecycle_state: superseded`, `status: historical`, revision 1 and three roots. The import added evidence to the record's history; it did not reopen it.

## Expected results

| Question | Returned (status, state) | Not returned |
|---|---|---|
| Now, production on 2.3, after step 2 | failure (current, active) | merge |
| Now, after step 4 or step 6 | merge, deployment (current, active) | failure, investigation |
| As of 2 September, after step 4 | failure (historical, superseded) | merge, deployment |
| As of 2 September, after step 6 | failure (historical, superseded), investigation (historical, active) | merge, deployment |
| Everything, after step 6 | all four, each labelled | |

The merge and the deployment can carry `stale: true` once their last check is more than 90 days old; they are still returned, and the answers above do not change. Whether the failure's scope (prod, 2.3) overlaps the deployment's (prod, 2.4) is not part of this story: the supersession is declared in step 4.
