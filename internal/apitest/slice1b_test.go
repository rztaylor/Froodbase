//go:build integration

package apitest

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/wire"
)

// TestSlice1bExit is the slice 1b exit scenario of
// docs/planning/07-local-mvp.md, driven through internal/client exactly as
// froodbase-mcp drives the API, with the calls the froodbase-contribute
// skill teaches Claude to build (development guide walkthrough steps 4 to 7
// and 12 to 14):
//
//   - a person's plain-language finding about a work item is contributed
//     with only what the request gives, and the server fills in the rest;
//   - an agent records an observation from an az command against the
//     import tool's web app, then an inference derived from both
//     observations: active, labelled inference, carrying one root from each
//     source and no citations of its own;
//   - the person adds a statement of their own, which stays below the
//     observation threshold;
//   - a second work item is added to the ticket observation under links,
//     and the dreamer passes the new root on to the inference;
//   - the findings of the committed sample document are recorded citing
//     its sections, deriving from the ticket observation;
//   - reading back shows every claim's sources: its citations, or the
//     records it was derived from, each of which is readable in turn, through
//     get_record, get_history and search.
//
// Nothing here calls the store: the only path in is the client.
func TestSlice1bExit(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	// 1. A plain-language finding: search first, then only what was said.
	if got := searchIDs(t, bob, "Import Tool"); len(got) != 0 {
		t.Fatalf("search before creating = %v, want nothing", got)
	}
	const workItem = "https://dev.azure.com/org/p/_workitems/edit/418"
	plain, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-callbacks", Rationale: "found during INC-12",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{{Ref: "wi", SourceURI: workItem, SourceRevision: "12"}},
		Assertions: []wire.AssertionInput{{Ref: "fails", Kind: "observation", Subject: "tool",
			BodyMarkdown: "Callbacks on 2.3 fail after a 30 second timeout", Evidence: []string{"wi"}}},
	})
	if err != nil {
		t.Fatalf("plain finding refused: %v", err)
	}
	tool, fails := written(plain)["tool"].ID, written(plain)["fails"]
	if fails.Kind != "observation" || fails.LifecycleState != "active" || fails.Support == nil || fails.Support.Roots != 1 || !fails.Support.MeetsThreshold {
		t.Fatalf("ticket observation = %+v (support %+v), want active with one root meeting its threshold", fails, fails.Support)
	}

	// 2. A fact the agent found by running az against the web app.
	if got := searchIDs(t, bob, "Import Tool"); len(got) == 0 || got[0] != tool {
		t.Fatalf("search(Import Tool) = %v, want the entity %s first", got, tool)
	}
	const (
		webApp = "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool"
		az     = "az webapp config appsettings list --ids /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool --query \"[?name=='CALLBACK_TIMEOUT_SECONDS'].value\" -o tsv"
	)
	ran := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	checked, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-timeout-setting", Rationale: "checked during INC-12",
		Evidence: []wire.EvidenceInput{{Ref: "run", SourceURI: webApp, Locator: az, Excerpt: "30", ObservedAt: &ran}},
		Assertions: []wire.AssertionInput{{Ref: "setting", Kind: "observation", Subject: tool, Evidence: []string{"run"},
			BodyMarkdown: "Production sets the import tool's callback timeout to 30 seconds", Scope: &wire.Scope{Environment: "prod"}}},
	})
	if err != nil {
		t.Fatalf("command observation refused: %v", err)
	}
	setting := written(checked)["setting"]
	if setting.Support == nil || setting.Support.Roots != 1 || !setting.Support.MeetsThreshold || !reflect.DeepEqual(rootKinds(setting.Support), map[string]int{"command": 1}) {
		t.Fatalf("command observation support = %+v, want one command root meeting its threshold", setting.Support)
	}

	// 3. The agent reads both, then records what it concludes from them.
	for _, id := range []string{fails.ID, setting.ID} {
		if rec, err := bob.GetRecord(ctx, wire.GetRecordInput{ID: id}); err != nil || rec.Kind != "observation" || len(rec.Citations) != 1 {
			t.Fatalf("get_record(%s) = %+v, %v", id, rec, err)
		}
	}
	concluded, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-cause", Rationale: "concluded during INC-12",
		Assertions: []wire.AssertionInput{{Ref: "cause", Kind: "inference", Subject: tool, DerivedFrom: []string{fails.ID, setting.ID},
			BodyMarkdown: "Callbacks on 2.3 fail because each retry waits out the 30 second callback timeout production sets"}},
	})
	if err != nil {
		t.Fatalf("inference refused: %v", err)
	}
	cause := written(concluded)["cause"]
	// One root from each source: a ticket and a command run are independent,
	// so the inference meets the default inference threshold of 2.
	if cause.Kind != "inference" || cause.LifecycleState != "active" || cause.Support == nil || cause.Support.Roots != 2 || cause.Support.Score != 2 ||
		!reflect.DeepEqual(rootKinds(cause.Support), map[string]int{"ticket": 1, "command": 1}) {
		t.Fatalf("inference written = %+v (support %+v), want active, labelled inference, one ticket and one command root", cause, cause.Support)
	}
	sources := sortedIDs(fails.ID, setting.ID)
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: cause.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Kind != "inference" || !rec.Derived || !reflect.DeepEqual(sortedIDs(rec.DerivedFrom...), sources) || len(rec.Citations) != 0 {
		t.Fatalf("get_record(inference) = %+v, want an inference derived from both observations", rec)
	}

	// 4. The person's own knowledge, as their statement.
	said := ran.Add(20 * time.Minute)
	stated, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-september", Rationale: "bob's own knowledge",
		Evidence: []wire.EvidenceInput{{Ref: "said", SourceKind: "statement", Excerpt: "We raised the callback timeout to 30 seconds in September.", ObservedAt: &said}},
		Assertions: []wire.AssertionInput{{Ref: "raised", Kind: "observation", Subject: tool, Evidence: []string{"said"},
			BodyMarkdown: "The callback timeout was raised to 30 seconds in September 2026"}},
	})
	if err != nil {
		t.Fatalf("statement refused: %v", err)
	}
	raised := written(stated)["raised"]
	if s := raised.Support; s == nil || s.Roots != 1 || s.Score != 0.5 || s.MeetsThreshold {
		t.Fatalf("statement observation support = %+v, want one statement root weighing 0.5, below threshold", s)
	}

	// 5. A second work item shows the same failure: a new independent root
	// on the ticket observation, which the dreamer passes on.
	corroborated, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "bob-inc-12-wi-419", Rationale: "the same failure in another work item",
		Evidence: []wire.EvidenceInput{{Ref: "wi419", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/419", SourceRevision: "3"}},
		Links:    []wire.LinkInput{{Claim: fails.ID, Evidence: []string{"wi419"}}},
	})
	if err != nil {
		t.Fatalf("link refused: %v", err)
	}
	if len(corroborated.Linked) != 1 || corroborated.Linked[0].ID != fails.ID || corroborated.Linked[0].RootsAdded != 1 ||
		corroborated.Linked[0].Revision != 1 || corroborated.Linked[0].Support.Roots != 2 {
		t.Fatalf("linked = %+v, want the ticket observation gaining one root at revision 1", corroborated.Linked)
	}
	WaitFor(t, func() bool {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: cause.ID})
		return err == nil && rec.Support != nil && rec.Support.Roots == 3
	}, 60*time.Second, "the inference gaining the work item's root")
	rec, err = carol.GetRecord(ctx, wire.GetRecordInput{ID: cause.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Revision != 1 || rec.Support.Score != 3 || !rec.Support.MeetsThreshold || !reflect.DeepEqual(rootKinds(rec.Support), map[string]int{"ticket": 2, "command": 1}) {
		t.Errorf("strengthened inference = revision %d, support %+v; want revision 1, two ticket roots and one command root", rec.Revision, rec.Support)
	}
	hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: cause.ID})
	if err != nil {
		t.Fatal(err)
	}
	var histSources []string
	for _, d := range hist.Derivations {
		histSources = append(histSources, d.SourceID)
	}
	if len(hist.Revisions) != 1 || !reflect.DeepEqual(sortedIDs(histSources...), sources) || len(hist.SupportEvents) != 3 {
		t.Fatalf("get_history(inference) = %d revisions, derivations %v, %d support events; want one revision, both sources, three events", len(hist.Revisions), histSources, len(hist.SupportEvents))
	}
	if e := hist.SupportEvents[2]; e.Type != "root_added" || e.JobName != "reconsider_derived" || e.ActorUserID != "" || e.Rationale != "inherited from "+fails.ID {
		t.Errorf("propagated event = %+v, want root_added by reconsider_derived, inherited from %s", e, fails.ID)
	}

	// 6. The findings of the committed sample document, citing its sections.
	if got := searchIDs(t, bob, "callbacks timeout"); !contains(got, fails.ID) {
		t.Fatalf("search(callbacks timeout) = %v, want the ticket observation %s", got, fails.ID)
	}
	doc, err := bob.Contribute(ctx, sampleDocumentFindings(tool, fails.ID))
	if err != nil {
		t.Fatalf("document findings refused: %v", err)
	}
	docRefs := written(doc)
	if s := docRefs["drops"].Support; s == nil || s.Roots != 1 || s.MeetsThreshold {
		t.Errorf("deduction resting on the document alone = %+v, want one root below threshold", s)
	}
	// The cause the document gives inherits both work items from the ticket
	// observation, plus the document.
	if s := docRefs["cause"].Support; s == nil || s.Roots != 3 || !reflect.DeepEqual(rootKinds(s), map[string]int{"ticket": 2, "repository": 1}) {
		t.Errorf("document's cause support = %+v, want two ticket roots and the document", s)
	}
	for ref, section := range map[string]string{"measured": "Measurements", "attempts": "Measurements", "cause": "Analysis", "drops": "Analysis", "cap": "Decision"} {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: docRefs[ref].ID})
		if err != nil {
			t.Fatal(err)
		}
		if len(rec.Citations) != 1 || rec.Citations[0].SourceURI != sampleDoc || rec.Citations[0].SourceRevision != sampleCommit ||
			rec.Citations[0].Locator != section || rec.Citations[0].RootEvidenceID != docRefs["doc"].ID {
			t.Errorf("%s citations = %+v, want the document at %s, section %s", ref, rec.Citations, sampleCommit, section)
		}
	}

	// 7. Reading back shows every record's sources: each claim cites
	// evidence, names the claims it was derived from, or both, and every
	// source is itself readable.
	claims := map[string]string{
		"ticket observation": fails.ID, "command observation": setting.ID, "inference": cause.ID, "statement": raised.ID,
	}
	for _, ref := range []string{"measured", "attempts", "cause", "drops", "cap"} {
		claims["document "+ref] = docRefs[ref].ID
	}
	read := map[string]wire.Record{}
	for name, id := range claims {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: id})
		if err != nil {
			t.Fatalf("get_record(%s) = %v", name, err)
		}
		if len(rec.Citations) == 0 && len(rec.DerivedFrom) == 0 {
			t.Errorf("%s has no sources: %+v", name, rec)
		}
		for _, c := range rec.Citations {
			if c.EvidenceID == "" || c.SourceKind == "" || c.SourceURI == "" || c.SourceRevision == "" || c.AccessClass != "area" {
				t.Errorf("%s citation = %+v, want kind, URI, revision and access class", name, c)
			}
		}
		for _, src := range rec.DerivedFrom {
			if _, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: src}); err != nil {
				t.Errorf("%s source %s: %v", name, src, err)
			}
		}
		read[name] = rec
	}
	ticketCites := read["ticket observation"].Citations
	if len(ticketCites) != 2 || ticketCites[0].SourceKind != "ticket" || ticketCites[1].SourceKind != "ticket" {
		t.Errorf("ticket observation citations = %+v, want both work items", ticketCites)
	}
	if c := read["command observation"].Citations; len(c) != 1 || c[0].SourceKind != "command" || c[0].SourceURI != webApp || c[0].Locator != az ||
		c[0].Excerpt != "30" || c[0].ObservedAt == nil || !c[0].ObservedAt.Equal(ran) {
		t.Errorf("command observation citations = %+v, want the az run with its command, output and time", c)
	}
	if c := read["statement"].Citations; len(c) != 1 || c[0].SourceKind != "statement" || c[0].SourceURI != "person://bob" || !strings.Contains(c[0].Excerpt, "September") {
		t.Errorf("statement citations = %+v, want bob's statement", c)
	}

	// Search returns the same sources with each result.
	found, err := carol.Search(ctx, wire.SearchInput{Query: "callback timeout", Area: "demo", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	hits := map[string]wire.Record{}
	for _, r := range found.Results {
		hits[r.Record.ID] = r.Record
	}
	for _, name := range []string{"ticket observation", "command observation", "inference", "statement"} {
		hit, ok := hits[claims[name]]
		if !ok {
			t.Errorf("search(callback timeout) did not return the %s", name)
			continue
		}
		if !reflect.DeepEqual(sortedIDs(hit.DerivedFrom...), sortedIDs(read[name].DerivedFrom...)) || len(hit.Citations) != len(read[name].Citations) {
			t.Errorf("search result for the %s = derived from %v, %d citations; get_record has %v, %d", name, hit.DerivedFrom, len(hit.Citations), read[name].DerivedFrom, len(read[name].Citations))
		}
	}
}

// rootKinds maps each source kind in s to its root count.
func rootKinds(s *wire.Support) map[string]int {
	m := map[string]int{}
	for _, r := range s.RootsBySourceKind {
		m[r.SourceKind] = r.Roots
	}
	return m
}
