package domain

// LifecycleState says whether a record is in force (TR-40): active,
// disputed, superseded or retracted. Every record enters active. It is
// independent of the record's epistemic kind, of its support and of its
// validity: an active record whose validity has ended is correct history.
// The transitions and who may make them arrive with the lifecycle state
// machine; this is the vocabulary the mode predicate reads.
type LifecycleState string

// The four lifecycle states.
const (
	LifecycleActive     LifecycleState = "active"     // in force
	LifecycleDisputed   LifecycleState = "disputed"   // in force, contradicted by an overlapping record
	LifecycleSuperseded LifecycleState = "superseded" // replaced by a declared correction
	LifecycleRetracted  LifecycleState = "retracted"  // withdrawn with a reason
)

// LifecycleStates lists the states in a stable order.
func LifecycleStates() []LifecycleState {
	return []LifecycleState{LifecycleActive, LifecycleDisputed, LifecycleSuperseded, LifecycleRetracted}
}

// Valid reports whether s is one of the four states.
func (s LifecycleState) Valid() bool {
	for _, c := range LifecycleStates() {
		if c == s {
			return true
		}
	}
	return false
}

// InForce reports whether the state is active or disputed: the states a
// current answer may hold. A disputed record is always labelled disputed.
func (s LifecycleState) InForce() bool {
	return s == LifecycleActive || s == LifecycleDisputed
}

// ParseLifecycleState returns the state named by s, or ErrUnknownLifecycleState.
func ParseLifecycleState(s string) (LifecycleState, error) {
	st := LifecycleState(s)
	if !st.Valid() {
		return "", ErrUnknownLifecycleState
	}
	return st, nil
}
