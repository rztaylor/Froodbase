package domain

import "time"

// Contribution is one act of contributing: who asserted a set of records,
// through which agent and client, as part of which investigation, and when
// (PR-10). It is the provenance episode of the architecture; the records it
// asserted are linked to it separately. Its RecordedAt is when Froodbase was
// told and is kept apart from any validity date on the records.
type Contribution struct {
	ID               ID
	AreaID           ID
	ActorUserID      ID     // the person, always present
	ActorAgentID     ID     // the agent acting for them, if any
	Client           string // the calling client and version
	InvestigationRef string // the investigation or conversation this came from, if any
	RecordedAt       time.Time
}

// Validate checks that the contribution has an id, an area, a human actor
// and a recording time.
func (c Contribution) Validate() error {
	if err := c.ID.Validate(); err != nil {
		return fieldErr("contribution id", err)
	}
	if err := c.AreaID.Validate(); err != nil {
		return fieldErr("area id", err)
	}
	if err := c.ActorUserID.Validate(); err != nil {
		return fieldErr("actor user id", err)
	}
	if c.RecordedAt.IsZero() {
		return fieldErr("recorded_at", ErrNoRecordedAt)
	}
	return nil
}
