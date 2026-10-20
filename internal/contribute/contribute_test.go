package contribute

import (
	"errors"
	"strings"
	"testing"

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
