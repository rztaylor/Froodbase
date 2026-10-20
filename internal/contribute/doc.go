// Package contribute turns a contribute call into records and writes them.
//
// It owns the translation of the tool input (wire.ContributeInput) into
// domain records with generated identifiers, the resolution of the caller's
// refs to ids (new records in the same call or existing records in the
// area), the role check (contributor in the area, refusals audited), the
// idempotency contract (TR-15: a request hash per key, a stored result
// replayed on retry, a different request under the same key refused) and the
// shaping of the result. The write itself, with revisions, audit, support
// events and the index job, is internal/store's; evidence validation and the
// support projection are internal/domain's. Entity resolution and conflict
// classification join here in later slices. Depends on internal/store,
// internal/domain, internal/authz and internal/wire.
package contribute
