//go:build integration

package apitest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// TestSlice1Exit is the slice 1 exit scenario of docs/planning/07-local-mvp.md,
// driven through internal/client exactly as froodbase-mcp drives the API:
// contribute an entity with an observation and shareable evidence, search
// for it by name and by a word in its body, read its history; a user
// without the area sees nothing and an audit event records the refusal;
// the index job runs within seconds of the write.
func TestSlice1Exit(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol, dave := env.Client("bob"), env.Client("carol"), env.Client("dave")

	// Who am I: the forwarding path and grants.
	me, err := bob.Whoami(ctx)
	if err != nil || me.UserID != "bob" || me.Grants["demo"][0] != "contributor" {
		t.Fatalf("whoami = %+v, %v", me, err)
	}

	// Contribute: one entity, one observation, two evidence records sharing a root.
	in := wire.ContributeInput{
		Area: "demo", IdempotencyKey: "slice1-1", Rationale: "seen during INC-1", InvestigationRef: "INC-1",
		Entities: []wire.EntityInput{{Ref: "tool", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence: []wire.EvidenceInput{
			{Ref: "ticket", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/418", SourceRevision: "2026-10-20T10:00:00Z", AccessClass: "area"},
			{Ref: "quote", SourceKind: "document", SourceURI: "https://contoso.sharepoint.com/sites/eng/runbook.docx", SourceRevision: "v7", Locator: "section 3", AccessClass: "area", Root: "ticket"},
		},
		Observations: []wire.ObservationInput{{
			Ref: "obs", Subject: "tool", BodyMarkdown: "Callback failure reproduced on version 2.3", Scope: &wire.Scope{Environment: "prod", VersionMin: "2.3", VersionMax: "2.3"},
			Evidence: []string{"ticket", "quote"},
		}},
	}
	out, err := bob.Contribute(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	var obs, tool wire.Written
	for _, r := range out.Records {
		switch r.Ref {
		case "obs":
			obs = r
		case "tool":
			tool = r
		}
	}
	if obs.ID == "" || obs.LifecycleState != "active" || obs.Support == nil || obs.Support.Roots != 1 || !obs.Support.MeetsThreshold {
		t.Fatalf("observation = %+v", obs)
	}
	replay, err := bob.Contribute(ctx, in)
	if err != nil || !replay.Replayed || replay.ContributionID != out.ContributionID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}

	// Search by exact name, by a near-miss name and by a word in the body.
	byName, err := carol.Search(ctx, wire.SearchInput{Query: "Import Tool"})
	if err != nil || len(byName.Results) == 0 || byName.Results[0].Record.ID != tool.ID || byName.Results[0].Match.Candidate {
		t.Fatalf("search by name = %+v, %v", byName, err)
	}
	nearMiss, err := carol.Search(ctx, wire.SearchInput{Query: "Improt Tool"})
	if err != nil || len(nearMiss.Results) == 0 || nearMiss.Results[0].Record.ID != tool.ID || !nearMiss.Results[0].Match.Candidate {
		t.Fatalf("search by near-miss = %+v, %v", nearMiss, err)
	}
	byBody, err := carol.Search(ctx, wire.SearchInput{Query: "callback"})
	if err != nil {
		t.Fatal(err)
	}
	var hit *wire.SearchResult
	for i := range byBody.Results {
		if byBody.Results[i].Record.ID == obs.ID {
			hit = &byBody.Results[i]
		}
	}
	if hit == nil || hit.Match.Channel != wire.ChannelFullText || hit.Record.Kind != "observation" || hit.Record.Support == nil || len(hit.Record.Citations) != 2 || !hit.Record.ValidityUnknown {
		t.Fatalf("search by body = %+v", hit)
	}

	// Read back and history.
	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: obs.ID})
	if err != nil || rec.Subject != tool.ID || rec.Scope == nil || rec.Scope.Environment != "prod" || rec.Area != "demo" {
		t.Fatalf("get_record = %+v, %v", rec, err)
	}
	hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: obs.ID})
	if err != nil || len(hist.Revisions) != 1 || hist.Revisions[0].ActorUserID != "bob" || hist.Revisions[0].Rationale != "seen during INC-1" || len(hist.SupportEvents) != 1 {
		t.Fatalf("get_history = %+v, %v", hist, err)
	}

	// A reader may not contribute; the refusal is audited.
	_, err = carol.Contribute(ctx, wire.ContributeInput{Area: "demo", IdempotencyKey: "c1", Entities: []wire.EntityInput{{Ref: "x", Type: "team", Names: []wire.Name{{Name: "Platform"}}}}})
	if err == nil || !strings.Contains(err.Error(), "contributor role required") {
		t.Fatalf("reader contribute err = %v", err)
	}
	// A user without the area sees nothing: not found, empty search.
	if _, err := dave.GetRecord(ctx, wire.GetRecordInput{ID: obs.ID}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("stranger get_record err = %v", err)
	}
	none, err := dave.Search(ctx, wire.SearchInput{Query: "Import Tool"})
	if err != nil || len(none.Results) != 0 {
		t.Fatalf("stranger search = %+v, %v", none, err)
	}
	demoID, _ := env.Areas.AreaID(ctx, "demo")
	events, err := env.DB.App.AuditEventsForArea(ctx, store.Caller{UserID: "carol", AreaIDs: []string{string(demoID)}}, demoID)
	if err != nil {
		t.Fatal(err)
	}
	refused := false
	for _, e := range events {
		if e.EventType == "contribution_refused" && e.ActorUserID == "carol" {
			refused = true
		}
	}
	if !refused {
		t.Fatal("no audit event for the refused contribution")
	}

	// The index job runs within seconds of the write.
	WaitFor(t, func() bool {
		s, err := env.Staleness(ctx)
		return err == nil && s.Total == 2 && s.Stale == 0
	}, 30*time.Second, "index jobs to index both records")
	if is, err := env.DB.App.GetIndexState(ctx, domain.ID(obs.ID)); err != nil || is.Revision != 1 {
		t.Fatalf("index state = %+v, %v", is, err)
	}
	if _, err := env.DB.App.GetIndexState(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown record index state err = %v", err)
	}
}
