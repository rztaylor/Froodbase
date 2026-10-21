//go:build integration

package apitest

import (
	"context"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// TestNewRootStrengthensDeductions is the strengthening half of
// reconsideration (FRB-52) as a reader sees it through internal/client: an
// observation, an inference derived from it and a decision derived from the
// inference, each with one root and the inference below its threshold. A
// new independent root linked to the observation (through the store here;
// TestEvidenceForExistingClaim does it through contribute) reaches the inference and
// the decision through the dreamer; get_record shows two roots and the
// inference now meets its threshold, and get_history shows the root_added
// appended by the job, inherited from the direct source, with no new
// revision.
func TestNewRootStrengthensDeductions(t *testing.T) {
	ctx := context.Background()
	env := Start(t)
	bob, carol := env.Client("bob"), env.Client("carol")

	out, err := bob.Contribute(ctx, wire.ContributeInput{
		Area: "demo", IdempotencyKey: "strengthen-1", Rationale: "seen during INC-9",
		Entities: []wire.EntityInput{{Ref: "gw", Type: "service", Names: []wire.Name{{Name: "Edge Gateway"}}}},
		Evidence: []wire.EvidenceInput{{Ref: "ticket", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/912", SourceRevision: "2", AccessClass: "area"}},
		Assertions: []wire.AssertionInput{
			{Ref: "obs", Kind: "observation", Subject: "gw", BodyMarkdown: "The edge gateway returned 503s during the nightly batch", Evidence: []string{"ticket"}},
			{Ref: "inf", Kind: "inference", Subject: "gw", BodyMarkdown: "The edge gateway is undersized for the nightly batch", DerivedFrom: []string{"obs"}},
			{Ref: "dec", Kind: "decision", Subject: "gw", BodyMarkdown: "Scale the edge gateway before the next batch", DerivedFrom: []string{"inf"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	refs := written(out)
	obs, inf, dec := refs["obs"], refs["inf"], refs["dec"]
	if inf.Support == nil || inf.Support.Roots != 1 || inf.Support.MeetsThreshold {
		t.Fatalf("inference written = %+v, want one root below threshold", inf.Support)
	}

	areaID, err := env.Areas.AreaID(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	caller := store.Caller{UserID: "bob", AreaIDs: []string{string(areaID)}}
	logs := domain.Evidence{
		ID: domain.ID(store.NewID()), AreaID: areaID, SourceKind: domain.SourceRepository, SourceURI: "https://github.com/org/edge/blob/main/capacity.md",
		SourceRevision: "9f1c2e7", SourceLocator: "L10-L24", AccessClass: domain.AccessArea, RecordedAt: time.Now(),
	}
	if _, err := env.DB.App.LinkEvidence(ctx, caller, domain.ID(obs.ID), []domain.Evidence{logs}, "capacity notes"); err != nil {
		t.Fatal(err)
	}

	WaitFor(t, func() bool {
		rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: dec.ID})
		return err == nil && rec.Support != nil && rec.Support.Roots == 2
	}, 60*time.Second, "the decision gaining the observation's new root")

	rec, err := carol.GetRecord(ctx, wire.GetRecordInput{ID: inf.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec.Support == nil || rec.Support.Roots != 2 || rec.Support.Score != 2 || !rec.Support.MeetsThreshold || rec.Revision != 1 {
		t.Fatalf("get_record(inference) = revision %d, support %+v; want two roots meeting the threshold at revision 1", rec.Revision, rec.Support)
	}
	for id, from := range map[string]string{inf.ID: obs.ID, dec.ID: inf.ID} {
		hist, err := carol.GetHistory(ctx, wire.GetHistoryInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if len(hist.Revisions) != 1 || len(hist.SupportEvents) != 2 {
			t.Fatalf("get_history(%s) = %d revisions, %d support events; want 1 and 2", id, len(hist.Revisions), len(hist.SupportEvents))
		}
		e := hist.SupportEvents[1]
		if e.Type != "root_added" || len(e.EvidenceRootIDs) != 1 || e.EvidenceRootIDs[0] != string(logs.ID) || e.JobName != "reconsider_derived" ||
			e.ActorUserID != "" || e.Rationale != "inherited from "+from {
			t.Errorf("propagated event of %s = %+v; want the new root added by reconsider_derived, inherited from %s", id, e, from)
		}
	}
}
