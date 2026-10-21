# Sample design note: import tool callback retries

> A fictional document for the worked example in the [development guide](../ops/development.md#recording-the-findings-in-a-document). The import tool, the callback gateway and the work item are made up. Cite this file at a commit, never at `main`.

Status: agreed on 14 October 2026 by the import team.

## Background

Import tool 2.3 changed how callbacks are retried. Work item 418 reports that callbacks on 2.3 fail after a 30 second timeout.

## Measurements

In staging, on version 2.3.0, a callback took 30.2 seconds on average over 200 imports. On 2.2.4 the same imports took 1.1 seconds.

The callback gateway gives up on a callback after three failed attempts.

## Analysis

The 2.3 retry waits out the full 30 second timeout before each retry, which explains the measured delay.

Because the gateway gives up after three attempts, slow retries on 2.3 probably make it drop callbacks under load. We have not measured this.

## Decision

From version 2.3.2 the callback timeout is capped at 5 seconds, and retries back off from 1 second.

## Open questions

- Do some customers see the same delay on 2.2?
- Should the gateway allow more attempts?
