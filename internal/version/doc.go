// Package version exposes the build identity shared by every Froodbase binary.
//
// It owns the version string, commit and build date injected at link time, and
// the compatibility check a client binary performs against a server's minimum
// supported client version. It must not import any other Froodbase package;
// every binary and the MCP version handshake depend on it, nothing depends on
// them from here.
package version
