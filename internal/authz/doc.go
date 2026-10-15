// Package authz owns the authorisation context of a request: who the caller
// is, which agent (if any) acts for them, and which (area, role) grants they
// hold.
//
// It defines the Context type, the Principal that a transport hands over once
// it has authenticated the caller, and the Builder interface that turns a
// Principal into a Context. The interface carries identity and grants only:
// no HTTP, basic-auth or token type appears in it, so the static-users
// implementation (authz/staticusers, the local MVP) and an Entra
// implementation (the cloud move) are interchangeable. Role implication
// (reviewer includes contributor includes reader) is decided here and nowhere
// else. Authentication itself, that is checking a password or verifying a
// token, is the transport's job, not this package's. Depends on nothing inside
// the module.
package authz
