// Package config loads the startup configuration of the three binaries from
// FROODBASE_ environment variables.
//
// It owns the variable names, their defaults, and validation of the values
// (presence, shape of the database URL, the review sweep's interval). It returns plain structs and never
// opens connections, reads files or logs values: a database URL carries a
// password and is only ever reported by its variable name. The cmd packages
// call it with os.LookupEnv and wire the result into the packages that need
// it. froodbase-mcp reads its API URL and credentials here too (LoadMCP).
// Depends on nothing inside the module.
package config
