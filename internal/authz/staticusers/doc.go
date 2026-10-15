// Package staticusers is the local MVP implementation of authz.Builder: a
// user list loaded once from a JSON file, each user with a bcrypt password
// hash and (area, role) grants.
//
// It owns the file format and its validation (unique usernames, bcrypt hashes
// only, known roles), password verification with a constant-time path that
// does not reveal whether a username exists, and the bcrypt helper used to
// create hashes. It never logs or returns passwords or hashes. The HTTP basic
// auth middleware in internal/httpserver calls Authenticate and then Build;
// nothing here knows about HTTP. Replaced by an Entra implementation for the
// cloud move without changes elsewhere. Depends on internal/authz only.
package staticusers
