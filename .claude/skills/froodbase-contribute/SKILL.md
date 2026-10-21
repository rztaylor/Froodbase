---
name: froodbase-contribute
description: Use when recording knowledge in Froodbase through its contribute tool. That covers a person saying in plain words what they learned, found, concluded or decided ("remember that...", "record this", "note in Froodbase that..."), an investigation turning up a finding that others could check, and a request to record the useful information in a document. Teaches how to turn the finding into a contribute change set without asking the person for fields. Search first, cite existing records, pick the evidence kind and the assertion kind, record deductions with their sources and commands as command evidence, add new sources to claims already recorded instead of recording them again, extract a document's findings with a citation to each section without sending the document, keep secrets out, and report the ids written.
---

# Recording findings in Froodbase

Froodbase is shared memory for people and agents. It holds entities (systems, services, teams), assertions about them, and the evidence behind each assertion that others can check. The person tells you what they learned in plain words. **Building the change set is your job, not theirs.** Never ask them for a field you can get from the conversation, from the sources you read or from the server. Prefer recording with provenance over refusing.

Tools (MCP server `froodbase`): `whoami`, `search`, `get_record`, `get_history`, `contribute`.

## Never send

- Reports, conversations, transcripts or whole documents. Send individual findings, each with its own citation (see Recording the findings in a document).
- Secrets. Replace any password, token, key, connection-string secret or signed URL with `***` in commands and output before you send them. Froodbase refuses anything that looks like a secret.
- Local paths (`/Users/...`, `C:\...`, `file://`) or URLs on this machine. They are refused because nobody else can check them.

## Recording a finding

1. **Area.** Call `whoami` once per session. Use the knowledge area where the caller holds `contributor`. If several fit, pick the one for the system concerned. Ask only when you genuinely cannot tell.
2. **Search first.** `search` the names of the systems involved, and a few words of the finding.
   - An exact name match: use its id as the `subject`. Do not create the entity again.
   - A near-miss candidate: read it with `get_record`. If it is plainly the same thing, use it. If it is ambiguous, ask the person one question that names both.
   - The same finding already recorded: do not record it again, because repetition earns nothing. If you have a source it does not cite yet, add that source to the recorded claim instead (see Adding evidence to something already recorded). If not, tell the person it exists, with its id.
   - The evidence you would cite already recorded (same source, revision and locator in a result's citations): cite its `evidence_id`. Do not record it again.
3. **Split into claims.** Each assertion holds one finding. Choose its `kind`, which never changes:
   - `observation`: what was seen in a source, a command's output or the person's experience. It cites evidence and is never derived.
   - `inference`: what was concluded. It names in `derived_from` the assertions it rests on (refs in this call or existing ids), cites evidence of its own, or both.
   - `decision`: what was approved or intended. It cites where it was decided, or is derived from what it rests on, or both.
4. **Evidence.** Cite a source the area's audience can check (see Evidence). Every observation needs at least one piece.
5. **Build the call** with only what you know (see What the server fills in). Set `scope` (environment, region, version_min/version_max, cluster, tenant) when the finding is limited to it. Set `valid_from`/`valid_to` only when a source states the dates. Never set them to today or to when you read the source. Put a short `rationale` (for example "found during INC-12") and `investigation_ref` when there is one.
6. **Idempotency key.** Mint one per change set: `<user>-<topic or investigation>-<short slug>`, for example `bob-inc-12-callback-retry`. After a timeout or error, retry with the same key and the same content. The result is then replayed and nothing is written twice. Never reuse a key for different content. A refused call writes nothing, so you may fix it and send it again under the same key.
7. **Contribute**, then report (see Reporting).

## Evidence

| What you have | `source_kind` | `source_uri` | Also |
|---|---|---|---|
| A work item, Jira or GitHub issue | inferred (`ticket`) | its https URL | `source_revision`: the work-item revision, or the date you read it |
| Code or a file in a repository | inferred (`repository`) | a permalink with the full commit SHA (`.../blob/<sha>/path`) | `locator`: line range or function. With the SHA in the URL, the revision is inferred. For a local checkout, build the URL from `git remote get-url origin` and `git rev-parse HEAD`, and only if that commit is pushed |
| A Confluence, SharePoint or Azure DevOps wiki page | inferred (`document`) | its https URL | `source_revision`: the page version, or the date you read it. `locator`: the section |
| A saved telemetry query | `telemetry_query` | its https URL | `source_revision`: the query snapshot or time window |
| Any other https page | name it (`document`, `ticket` and so on) | its https URL | `source_revision` |
| A command or query you ran | inferred (`command`) | what it observed: `azure:///subscriptions/...` (the ARM id), `k8s://<cluster>/<namespace>/<kind>/<name>`, `sql://<server>/<database>`, `host://<name>` | `locator`: the exact command, scrubbed. `observed_at`: when it ran (RFC 3339, UTC). `excerpt`: the output line you relied on, scrubbed |
| What the person knows from their own experience | `statement` (inferred from `person://`) | leave out. It becomes `person://<their user id>` | `excerpt`: what they said, in their words. `observed_at`: when they said it |

- `access_class` defaults to `area`. Set `restricted` when the source is limited to named people, for example a restricted ticket, a personal file or a security case. Set `organisation` only when everyone in the organisation can open the source. Never claim a wider class than the source allows.
- `root` names the evidence (ref or id) that this evidence restates, for example a document quoting a ticket. Corroboration then counts the original once.
- Each run of a command is its own evidence and its own root. Running it again later and seeing the same thing is real corroboration, so record the new run.

## Commands you ran

When you learn something by running `az`, `kubectl`, `sqlcmd`, `psql` or similar, record it as an observation that cites command evidence. Use the resource as `source_uri`, the exact command as `locator`, the run time as `observed_at` and the relevant output as `excerpt`, all scrubbed. Do not paste whole outputs. Quote the line the finding rests on.

## The person's own knowledge

When the person states something from their own experience and asks you to record it, that request is their agreement. Record an observation citing a `statement` with their words as the excerpt. You can only record the caller's own statements. For something a colleague said, cite where that colleague wrote it down (a ticket or a document). If no person is behind the call (you are acting alone), you cannot record a statement. A statement weighs less than an observed source, so a claim resting only on a statement is stored below its threshold. That is expected.

## Deductions

When you conclude something from records you read, or from what you just recorded, record it as an `inference` with `derived_from` naming those assertions. They can be refs in the same call (in any order) or existing ids. Add evidence of its own when you have it, for example the code that explains the cause. A deduction starts with the distinct evidence roots of its sources, stays labelled as an inference, and is often below its threshold at first. That is expected: do not add weak evidence just to lift it. Later evidence on its sources strengthens it automatically. Never record a conclusion as an observation, and never name `derived_from` on an observation. Deriving from another area, from an unknown id or in a loop is refused.

## Adding evidence to something already recorded

When a claim is already recorded and you have found a source it does not cite yet (another ticket, a second command run, a log, the code that shows it), add the source to that claim. Do not record the claim again. Use the claim's `id` from `search` or `get_record`, and list it under `links`, with the evidence as a ref in the same call or an existing `evidence_id`:

```json
{
  "area": "demo",
  "idempotency_key": "bob-inc-31-gateway-logs",
  "rationale": "gateway logs agree",
  "evidence": [{"ref": "logs", "source_uri": "https://github.com/org/edge/blob/0123456789abcdef0123456789abcdef01234567/capacity.md", "locator": "L10-L24"}],
  "links": [{"claim": "<the observation's id>", "evidence": ["logs"]}]
}
```

- The claim gains one root for each independent source it did not count yet. Its text and revision do not change, and deductions derived from it gain the root shortly afterwards.
- Evidence the claim already counts adds nothing: the same evidence again, a restatement of a source it cites (`root`), a source it already inherits, or another statement from the same person. The result then says `roots_added: 0`. Tell the person it was already counted rather than looking for ways to add more.
- `claim` is an id, of a claim in the area you contribute to. A claim in the same call takes its evidence in its own `evidence` list, not under `links`.
- You can add links in the same call as new records. A deduction in that call derived from a linked claim inherits the new roots at once.
- If what you found changes the claim (narrower scope, a different version, a correction), it is a new claim, not more evidence for the old one.

## Recording the findings in a document

When the person says "record the useful information in this document", read the document with your own tools and contribute its findings one by one. Never send the document, a summary of it or its sections. Froodbase only holds each finding, its citation and at most a short excerpt.

1. **Where it lives decides what you cite.**
   - **In a repository** (committed and pushed) or on a wiki page at a known version: the document is the source. For a local checkout, build the permalink from `git remote get-url origin` and `git log -1 --format=%H -- <path>`, and check that the commit is pushed (`git branch -r --contains <sha>`). Never cite `main` or a branch.
   - **Only on this computer** (a local file, a download, an unpushed draft): it cannot be cited, because nobody else can open it. Cite what it relied on instead: the tickets, pull requests and pages it names, the commands it quotes (re-run them if you can and cite your run), and the person's own knowledge as a `statement` if they agree to state it. Ask them once, listing those findings, before you record any statement for them.
2. **Search first**, for every system the document names and for its main findings (step 2 of Recording a finding). Use existing entities as subjects, so the document's systems join the records the area already has. A finding that is already recorded is not recorded again. If the document only restates a source that claim already cites (it quotes the same ticket), it adds nothing, so tell the person. If it has a source of its own (a measurement, a code reference), add it under `links`.
3. **Sort each statement in the document.**
   - What it reports seeing (a measurement, a log line, a configuration) is an `observation` citing its section.
   - What it concludes is an `inference` derived from the records it rests on (the observations in this call, or recorded ones), citing the section where it is concluded. A conclusion the document marks as a guess or unmeasured is still an inference. It just stays below its threshold.
   - What it decided or agreed is a `decision` citing the section, derived from what it rests on when the document says so. Set `valid_from` only when the document states the date.
   - Open questions, to-dos, opinions without a claim, and anything you cannot point to a section for are not findings. Do not record them. List them back to the person.
4. **One source, many sections.** Record the document once, with no `locator`, first in `evidence`. Then record one evidence per section you cite, with the section heading as `locator` (add a line range when the section is long) and `root` set to the document's ref. The document then counts as one root however many sections you cite, so two of its own findings never corroborate each other. A section that restates another source you cite (a ticket it quotes) names that source as its `root` instead. A `root` always names an original, and comes earlier in `evidence` than the records that name it. If the document at that commit is already recorded, cite its `evidence_id` as the root rather than recording it again.
5. **One change set** for the whole document, with one idempotency key (`<user>-<document slug>`) and a `rationale` naming the document. Each finding stays its own assertion.
6. **Report** as usual, and add what you did not record: the open questions, the findings already recorded, and for a local file, each finding with nothing checkable behind it, so the person can point you to a source or state it themselves. Never store such a finding with an invented citation.

A document in a repository, with one observation and the inference drawn from it plus one recorded observation:

```json
{
  "area": "demo",
  "idempotency_key": "bob-sample-design-note",
  "rationale": "findings of the sample design note",
  "evidence": [
    {"ref": "doc", "source_uri": "https://github.com/rztaylor/Froodbase/blob/<sha>/docs/dev/guides/sample-design-note.md"},
    {"ref": "measurements", "source_uri": "https://github.com/rztaylor/Froodbase/blob/<sha>/docs/dev/guides/sample-design-note.md", "locator": "Measurements", "root": "doc"},
    {"ref": "analysis", "source_uri": "https://github.com/rztaylor/Froodbase/blob/<sha>/docs/dev/guides/sample-design-note.md", "locator": "Analysis", "root": "doc"}
  ],
  "assertions": [
    {"ref": "measured", "kind": "observation", "subject": "<import tool id>", "body_markdown": "In staging, callbacks on 2.3.0 took 30.2 seconds on average, against 1.1 seconds on 2.2.4", "evidence": ["measurements"], "scope": {"environment": "staging", "version_min": "2.3.0", "version_max": "2.3.0"}},
    {"kind": "inference", "subject": "<import tool id>", "body_markdown": "The 2.3 retry waits out the full 30 second timeout before each retry", "derived_from": ["<recorded observation id>", "measured"], "evidence": ["analysis"]}
  ]
}
```

The development guide has the whole worked example ("Recording the findings in a document").

## What the server fills in

Leave these out. The server infers them, and spelling them out changes nothing:

- `ref` on any record that nothing else in the call refers to. Generated refs are `entity-1`, `evidence-2`, `assertion-3` and `observation-1`, numbered by position in their list. Give a ref only to what something else names (an entity used as a subject, evidence cited by an assertion, an assertion named in `derived_from`).
- `source_kind` for the URLs and schemes marked "inferred" in the table.
- `access_class`, which defaults to `area`.
- `source_revision` for a repository URL with a full commit SHA, for commands and for statements.
- A statement's `source_uri`.

Still required: `area`, `idempotency_key`, each entity's `type` (system, service, component, team, environment or incident) and `names`, each assertion's `kind`, `subject` and `body_markdown`, evidence for every observation, each link's `claim` and `evidence`, and the revision of any https source other than a pinned commit.

## Refusals

A refusal writes nothing and says what to fix. Fix the call and send it again.

| Message says | Do |
|---|---|
| `source_kind is required` | Name the kind from the table. |
| `source_revision is required` | Find the revision: the commit SHA (or a permalink), the work-item revision, the document version, or the date you read it. |
| `looks like it holds a secret` | Replace the value with `***` and send it again. |
| `already holds evidence for that source` | Find its id (`search` results carry `evidence_id` in their citations) and cite that id instead. |
| `cites no evidence` | Find a source, or record it as a deduction derived from what it rests on. If nothing is checkable, see When to ask. |
| `which is a ref in this call` | Cite the evidence in that new assertion's `evidence` list. `links` is only for claims already recorded. |
| `not the id of an assertion` or `a referenced record was not found` | `search` for the recorded claim and use its `id`. |
| `in another knowledge area` | Add evidence only to claims in the area you contribute to. Record the finding in that area's own terms instead. |
| `contributor role required` | Tell the person they cannot contribute to that area. Do not retry. |
| `froodbase_unavailable: true` | Carry on without Froodbase. Tell the person the findings were not recorded. |

## When to ask

Ask the person only when a claim has nothing checkable behind it. That means no source you read, no command you ran, nothing it can be derived from, and not their own statement. Do not store such a claim with an invented citation. List it back to them and ask whether they can point to a source, or whether they want to state it as their own knowledge. Also ask when you cannot tell which area or which existing entity is meant. Never ask for ids, kinds, revisions, access classes or keys.

## Reporting

After `contribute`, tell the person what was written. For each record, give its kind, a few words of what it says, its id, and for a claim its support (`roots`, `meets_threshold`). For each claim under `linked`, give its id, `roots_added` and its support now. Name anything you did not record and why. If you are writing a report, list the ids in it rather than sending the report to Froodbase.

## Example

The person says: "Remember this: import callbacks on 2.3 fail because the new retry waits out the full 30 second timeout. It's in work item 418, and the code is retryCallback in callback/retry.go." Earlier in the session you ran `kubectl` and saw production on 2.3.1.

After `whoami` (area `demo`) and a `search` for "Import Tool" that found nothing:

```json
{
  "area": "demo",
  "idempotency_key": "bob-inc-12-callback-retry",
  "rationale": "found during INC-12",
  "entities": [{"ref": "tool", "type": "service", "names": [{"name": "Import Tool"}]}],
  "evidence": [
    {"ref": "wi", "source_uri": "https://dev.azure.com/org/p/_workitems/edit/418", "source_revision": "12"},
    {"ref": "code", "source_uri": "https://github.com/org/import-tool/blob/0123456789abcdef0123456789abcdef01234567/callback/retry.go#L40-L58", "locator": "retryCallback"},
    {"ref": "run", "source_uri": "k8s://aks-prod/import/deployment/import-api",
     "locator": "kubectl -n import get deployment import-api -o jsonpath={.spec.template.spec.containers[0].image}",
     "excerpt": "importtool:2.3.1", "observed_at": "2026-10-21T10:00:00Z"}
  ],
  "assertions": [
    {"ref": "fails", "kind": "observation", "subject": "tool", "body_markdown": "Callbacks fail after a 30 second timeout on version 2.3", "evidence": ["wi"], "scope": {"version_min": "2.3", "version_max": "2.3"}},
    {"kind": "observation", "subject": "tool", "body_markdown": "Production runs import tool 2.3.1", "evidence": ["run"], "scope": {"environment": "prod"}},
    {"kind": "inference", "subject": "tool", "body_markdown": "The 2.3 retry change makes callbacks wait for the full timeout before retrying", "derived_from": ["fails"], "evidence": ["code"]}
  ]
}
```

The server infers `ticket`, `repository` and `command`, the `area` access class and the code's revision, and names the unlabelled records `assertion-2` and `assertion-3`. Report the six ids, with each claim's support. The inference inherits the work item's root and adds the code's root.
