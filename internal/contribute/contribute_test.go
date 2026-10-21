package contribute

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

func good() wire.ContributeInput {
	return wire.ContributeInput{
		Area: "demo", IdempotencyKey: "k1",
		Entities:     []wire.EntityInput{{Ref: "e1", Type: "service", Names: []wire.Name{{Name: "Import Tool"}}}},
		Evidence:     []wire.EvidenceInput{{Ref: "ev1", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/1", SourceRevision: "2026-10-20", AccessClass: "area"}},
		Observations: []wire.ObservationInput{{Ref: "o1", Subject: "e1", BodyMarkdown: "Callback failed on 2.3", Evidence: []string{"ev1"}}},
	}
}

func TestValidateInput(t *testing.T) {
	if err := validateInput(good()); err != nil {
		t.Fatalf("good input rejected: %v", err)
	}
	cases := map[string]func(*wire.ContributeInput){
		"no area":                 func(in *wire.ContributeInput) { in.Area = " " },
		"no key":                  func(in *wire.ContributeInput) { in.IdempotencyKey = "" },
		"empty change set":        func(in *wire.ContributeInput) { in.Entities, in.Evidence, in.Observations = nil, nil, nil },
		"entity without ref":      func(in *wire.ContributeInput) { in.Entities[0].Ref = "" },
		"entity without names":    func(in *wire.ContributeInput) { in.Entities[0].Names = nil },
		"duplicate ref":           func(in *wire.ContributeInput) { in.Evidence[0].Ref = "e1" },
		"observation no subject":  func(in *wire.ContributeInput) { in.Observations[0].Subject = "" },
		"observation no evidence": func(in *wire.ContributeInput) { in.Observations[0].Evidence = nil },
	}
	for name, mutate := range cases {
		in := good()
		mutate(&in)
		err := validateInput(in)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
		}
	}
	in := good()
	in.Observations[0].Evidence = nil
	if err := validateInput(in); err == nil || !strings.Contains(err.Error(), "PR-43") {
		t.Errorf("missing evidence message = %v, want the PR-43 reason", err)
	}
}

// withDeductions adds an inference derived from the observation and a
// decision derived from the inference, listed before it.
func withDeductions() wire.ContributeInput {
	in := good()
	in.Assertions = []wire.AssertionInput{
		{Ref: "d1", Kind: "decision", Subject: "e1", BodyMarkdown: "Retry callbacks before 2.4", DerivedFrom: []string{"i1"}},
		{Ref: "i1", Kind: "inference", Subject: "e1", BodyMarkdown: "Callbacks fail under load", DerivedFrom: []string{"o1", "00000000-0000-0000-0000-000000000001"}},
	}
	return in
}

func TestValidateDeductions(t *testing.T) {
	if err := validateInput(withDeductions()); err != nil {
		t.Fatalf("deductions rejected: %v", err)
	}
	ok := map[string]func(*wire.ContributeInput){
		"assertions only": func(in *wire.ContributeInput) {
			in.Observations = nil
			in.Assertions[1].DerivedFrom = []string{"6f1c9a52-3b0e-4f4e-9d61-2a7c5e8b1d40"}
		},
		"observation as an assertion": func(in *wire.ContributeInput) {
			in.Assertions = append(in.Assertions, wire.AssertionInput{Ref: "o2", Kind: "observation", Subject: "e1", BodyMarkdown: "Seen", Evidence: []string{"ev1"}})
		},
		"decision with evidence only": func(in *wire.ContributeInput) {
			in.Assertions[0].DerivedFrom, in.Assertions[0].Evidence = nil, []string{"ev1"}
		},
		"inference with evidence too":  func(in *wire.ContributeInput) { in.Assertions[1].Evidence = []string{"ev1"} },
		"duplicate source is harmless": func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"o1", "o1"} },
	}
	for name, mutate := range ok {
		in := withDeductions()
		mutate(&in)
		if err := validateInput(in); err != nil {
			t.Errorf("%s: rejected: %v", name, err)
		}
	}
	bad := map[string]struct {
		mutate func(*wire.ContributeInput)
		says   string
	}{
		"no kind":      {func(in *wire.ContributeInput) { in.Assertions[0].Kind = "" }, "kind"},
		"unknown kind": {func(in *wire.ContributeInput) { in.Assertions[0].Kind = "guess" }, "kind"},
		"no grounds":   {func(in *wire.ContributeInput) { in.Assertions[0].DerivedFrom = nil }, "cites no evidence and names no derived_from"},
		"observation without cites": {func(in *wire.ContributeInput) {
			in.Assertions[0] = wire.AssertionInput{Ref: "d1", Kind: "observation", Subject: "e1", BodyMarkdown: "x"}
		}, "PR-43"},
		"derived observation": {func(in *wire.ContributeInput) {
			in.Assertions[0].Kind, in.Assertions[0].Evidence = "observation", []string{"ev1"}
		}, "inference"},
		"derived from evidence":       {func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"ev1"} }, "entity or evidence ref"},
		"derived from entity":         {func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"e1"} }, "entity or evidence ref"},
		"cycle":                       {func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"d1"} }, "cycle involving d1, i1"},
		"self":                        {func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"i1"} }, "cycle"},
		"ref shared with observation": {func(in *wire.ContributeInput) { in.Assertions[0].Ref = "o1" }, "used twice"},
		"typo ref":                    {func(in *wire.ContributeInput) { in.Assertions[1].DerivedFrom = []string{"o2"} }, "neither the ref of an assertion in this call nor an assertion id"},
		"no subject":                  {func(in *wire.ContributeInput) { in.Assertions[1].Subject = " " }, "no subject"},
	}
	for name, tt := range bad {
		in := withDeductions()
		tt.mutate(&in)
		err := validateInput(in)
		if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.says) {
			t.Errorf("%s: err = %v, want ErrInvalidInput saying %q", name, err, tt.says)
		}
	}
}

func TestAssertionsOfKeepsObservationsFirst(t *testing.T) {
	got := assertionsOf(withDeductions())
	if len(got) != 3 || got[0].Ref != "o1" || got[0].Kind != "observation" || got[0].Evidence[0] != "ev1" || got[1].Ref != "d1" || got[2].Ref != "i1" {
		t.Errorf("assertionsOf() = %+v", got)
	}
}

func TestRequestHashIgnoresKeyOnly(t *testing.T) {
	a, b := good(), good()
	b.IdempotencyKey = "other"
	if requestHash(a) != requestHash(b) {
		t.Fatal("hash depends on the idempotency key")
	}
	b.Observations[0].BodyMarkdown += "!"
	if requestHash(a) == requestHash(b) {
		t.Fatal("hash ignores the content")
	}
}

// A statement records what the caller themself knows: its person is the
// human caller, filled in when left out; someone else's statement, or one
// with no person behind the call, is refused (PR-11).
func TestAttributeStatements(t *testing.T) {
	said := func(uri string) wire.ContributeInput {
		in := good()
		in.Evidence = append(in.Evidence, wire.EvidenceInput{Ref: "s1", SourceKind: "statement", SourceURI: uri, Excerpt: "The job runs nightly", AccessClass: "area"})
		return in
	}
	bob := &authz.Context{UserID: "bob"}

	in := said("")
	if err := attributeStatements(&in, bob); err != nil {
		t.Fatalf("own statement without a uri: %v", err)
	}
	if got := in.Evidence[1].SourceURI; got != "person://bob" {
		t.Errorf("filled uri = %q, want person://bob", got)
	}
	if in.Evidence[0].SourceURI != good().Evidence[0].SourceURI {
		t.Errorf("non-statement evidence changed: %q", in.Evidence[0].SourceURI)
	}

	in = said("Person://bob")
	if err := attributeStatements(&in, bob); err != nil || in.Evidence[1].SourceURI != "person://bob" {
		t.Errorf("own statement: %q, %v; want it as person://bob", in.Evidence[1].SourceURI, err)
	}

	in = said("person://carol")
	err := attributeStatements(&in, bob)
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "only record their own statements") {
		t.Errorf("someone else's statement: err = %v", err)
	}

	in = said("")
	err = attributeStatements(&in, &authz.Context{AgentID: "triage-agent"})
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "no person") {
		t.Errorf("agent without a person: err = %v", err)
	}

	in = said("https://example.com/bob")
	if err := attributeStatements(&in, bob); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("statement with a non-person uri: err = %v", err)
	}

	in = good()
	if err := attributeStatements(&in, &authz.Context{AgentID: "triage-agent"}); err != nil {
		t.Errorf("no statements, no person needed: %v", err)
	}
}

const (
	claimA = "6f1c9a52-3b0e-4f4e-9d61-2a7c5e8b1d40"
	claimB = "0b8e3c11-6a2d-4c5f-8e9a-1d2c3b4a5f60"
	evOld  = "9a7b6c5d-4e3f-4a1b-8c2d-3e4f5a6b7c8d"
)

// withLinks is new evidence for a claim already recorded, and an existing
// evidence id for another, with nothing else in the call.
func withLinks() wire.ContributeInput {
	return wire.ContributeInput{
		Area: "demo", IdempotencyKey: "k2",
		Evidence: []wire.EvidenceInput{{Ref: "ev1", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/2", SourceRevision: "3", AccessClass: "area"}},
		Links:    []wire.LinkInput{{Claim: claimA, Evidence: []string{"ev1"}}, {Claim: claimB, Evidence: []string{evOld}}},
	}
}

// Evidence for an existing claim names the claim by id and cites evidence
// refs from the call or ids from the area; anything else is refused before
// anything is written.
func TestValidateLinks(t *testing.T) {
	if err := validateInput(withLinks()); err != nil {
		t.Fatalf("links rejected: %v", err)
	}
	ok := map[string]func(*wire.ContributeInput){
		"links only, existing evidence": func(in *wire.ContributeInput) { in.Evidence, in.Links = nil, in.Links[1:] },
		"with a new claim too": func(in *wire.ContributeInput) {
			g := good()
			in.Entities, in.Observations = g.Entities, g.Observations
			in.Evidence = append(in.Evidence, wire.EvidenceInput{Ref: "ev0", SourceKind: "ticket", SourceURI: "https://dev.azure.com/org/p/_workitems/edit/1", SourceRevision: "1"})
			in.Observations[0].Evidence = []string{"ev0"}
		},
		"same claim twice": func(in *wire.ContributeInput) { in.Links[1].Claim = claimA },
	}
	for name, mutate := range ok {
		in := withLinks()
		mutate(&in)
		if err := validateInput(in); err != nil {
			t.Errorf("%s: rejected: %v", name, err)
		}
	}
	bad := map[string]struct {
		mutate func(*wire.ContributeInput)
		says   string
	}{
		"no claim":    {func(in *wire.ContributeInput) { in.Links[0].Claim = " " }, "names no claim"},
		"no evidence": {func(in *wire.ContributeInput) { in.Links[0].Evidence = nil }, "cites no evidence"},
		"claim is a ref in this call": {func(in *wire.ContributeInput) {
			g := good()
			in.Entities, in.Observations = g.Entities, g.Observations
			in.Observations[0].Evidence = []string{"ev1"}
			in.Links[0].Claim = "o1"
		}, "cite the evidence in that assertion"},
		"claim is evidence ref": {func(in *wire.ContributeInput) { in.Links[0].Claim = "ev1" }, "is a ref in this call"},
		"claim not an id":       {func(in *wire.ContributeInput) { in.Links[0].Claim = "callback-retry" }, "not the id of an assertion"},
		"evidence typo":         {func(in *wire.ContributeInput) { in.Links[0].Evidence = []string{"ev2"} }, "neither the ref of evidence in this call nor an evidence id"},
		"evidence is an entity": {func(in *wire.ContributeInput) {
			in.Entities = good().Entities
			in.Links[0].Evidence = []string{"e1"}
		}, "neither the ref of evidence in this call nor an evidence id"},
	}
	for name, tt := range bad {
		in := withLinks()
		tt.mutate(&in)
		err := validateInput(in)
		if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), tt.says) {
			t.Errorf("%s: err = %v, want ErrInvalidInput saying %q", name, err, tt.says)
		}
	}
}

// Links translate into the store's links by claim id, refs resolved to the
// new evidence's ids, in the order given, one link per claim with each
// evidence id once.
func TestBuildLinks(t *testing.T) {
	n := 0
	s := &Service{now: func() time.Time { return time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC) }, newID: func() string {
		n++
		return fmt.Sprintf("00000000-0000-0000-0000-%012d", n)
	}}
	in := withLinks()
	in.Links = append(in.Links, wire.LinkInput{Claim: claimA, Evidence: []string{evOld, "ev1"}})
	c, _, err := s.build(context.Background(), store.Caller{}, &authz.Context{UserID: "bob"}, "a0000000-0000-0000-0000-000000000000", in)
	if err != nil {
		t.Fatal(err)
	}
	newEv := c.Evidence[0].ID
	want := []store.EvidenceLink{
		{ClaimID: claimA, EvidenceIDs: []domain.ID{newEv, evOld}},
		{ClaimID: claimB, EvidenceIDs: []domain.ID{evOld}},
	}
	if !reflect.DeepEqual(c.Links, want) {
		t.Errorf("links = %+v, want %+v", c.Links, want)
	}
}

func TestRequestHashCoversLinks(t *testing.T) {
	a, b := withLinks(), withLinks()
	b.Links[0].Evidence = []string{evOld}
	if requestHash(a) == requestHash(b) {
		t.Fatal("hash ignores the links")
	}
}
