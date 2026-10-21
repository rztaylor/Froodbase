package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var ran = time.Date(2026, 10, 21, 9, 30, 0, 0, time.UTC)

// command is a command observation: what was observed, the command run
// (the locator) and when it ran.
func command(uri, cmd string) Evidence {
	at := ran
	return Evidence{
		ID: "ev1", AreaID: "a1", SourceKind: SourceCommand, SourceURI: uri, SourceRevision: ran.Format(time.RFC3339),
		SourceLocator: cmd, Excerpt: `"provisioningState": "Succeeded"`, AccessClass: AccessArea, ObservedAt: &at, RecordedAt: t1,
	}
}

// statement is what a named person said, and when.
func statement(uri, said string) Evidence {
	at := ran
	return Evidence{
		ID: "ev1", AreaID: "a1", SourceKind: SourceStatement, SourceURI: uri, SourceRevision: ran.Format(time.RFC3339),
		Excerpt: said, AccessClass: AccessArea, ObservedAt: &at, RecordedAt: t1,
	}
}

const armID = "azure:///subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool"

// A command observation cites the observed target in one of the provider
// forms, the command it ran and when it ran (TR-24, TR-61).
func TestCommandEvidenceValidate(t *testing.T) {
	az := "az webapp show --ids /subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg-import/providers/Microsoft.Web/sites/import-tool"
	tests := []struct {
		name string
		e    Evidence
		want error
	}{
		{"azure resource id", command(armID, az), nil},
		{"azure management group", command("azure:///providers/Microsoft.Management/managementGroups/mg-prod", "az account management-group show -n mg-prod"), nil},
		{"azure subscription", command("azure:///subscriptions/00000000-0000-0000-0000-000000000000", "az account show"), nil},
		{"k8s namespaced object", command("k8s://aks-prod-weu/import/deployment/import-api", "kubectl -n import get deployment import-api -o yaml"), nil},
		{"k8s cluster-scoped object", command("k8s://aks-prod-weu/node/aks-nodepool1-0", "kubectl get node aks-nodepool1-0"), nil},
		{"sql server and database", command("sql://sql-import.database.windows.net/importdb", "select count(*) from jobs where state = 'failed'"), nil},
		{"sql server with port", command("sql://sql-import.database.windows.net:1433/importdb", "select 1"), nil},
		{"generic host", command("host://vm-import-01.contoso.net", "systemctl status import-agent"), nil},

		// Each provider form has one shape.
		{"azure with a host", command("azure://subscriptions/0000/resourceGroups/rg", az), ErrURINotAllowed},
		{"azure without subscriptions or providers", command("azure:///resourceGroups/rg", az), ErrURINotAllowed},
		{"azure with only the subscriptions segment", command("azure:///subscriptions/", az), ErrURINotAllowed},
		{"k8s without a cluster", command("k8s:///import/deployment/import-api", "kubectl get deploy"), ErrURINotAllowed},
		{"k8s with only a kind", command("k8s://aks-prod-weu/deployment", "kubectl get deploy"), ErrURINotAllowed},
		{"k8s with too many segments", command("k8s://aks-prod-weu/import/apps/deployment/import-api", "kubectl get deploy"), ErrURINotAllowed},
		{"sql without a database", command("sql://sql-import.database.windows.net", "select 1"), ErrURINotAllowed},
		{"sql with a table", command("sql://sql-import.database.windows.net/importdb/jobs", "select 1"), ErrURINotAllowed},
		{"sql with a user", command("sql://reader@sql-import.database.windows.net/importdb", "select 1"), ErrURINotAllowed},
		{"host with a path", command("host://vm-import-01/var/log", "tail"), ErrURINotAllowed},
		{"host with a query", command("host://vm-import-01?x=1", "uptime"), ErrURINotAllowed},
		{"https is not a command target", command("https://portal.azure.com/#resource/x", az), ErrURINotAllowed},
		{"unknown scheme", command("aws://arn:aws:s3:::bucket", "aws s3 ls"), ErrURINotAllowed},
		{"empty target", command("", az), ErrURINotAllowed},

		// The contributor's own machine is not checkable by anyone else (PR-11).
		{"localhost", command("host://localhost", "uptime"), ErrLocalSource},
		{"loopback sql", command("sql://127.0.0.1/importdb", "select 1"), ErrLocalSource},
		{"loopback cluster", command("k8s://localhost/import/pod/p", "kubectl get pod p"), ErrLocalSource},
		{"local path", command("/var/log/import.log", "tail"), ErrLocalSource},

		// The command and the time it ran are what make it checkable.
		{"no command", command(armID, " "), ErrNoCommand},
		{"no observation time", func() Evidence { e := command(armID, az); e.ObservedAt = nil; return e }(), ErrNoObservedAt},
		{"output excerpt is optional", func() Evidence { e := command(armID, az); e.Excerpt = ""; return e }(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.e.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

// A statement cites the person who made it, what they said and when.
func TestStatementEvidenceValidate(t *testing.T) {
	tests := []struct {
		name string
		e    Evidence
		want error
	}{
		{"named person", statement("person://bob", "The import tool retries callbacks three times."), nil},
		{"person id with dots", statement("person://robert.taylor", "We moved the job to West Europe in March."), nil},
		{"no person", statement("person://", "something"), ErrURINotAllowed},
		{"a path after the person", statement("person://bob/notes", "something"), ErrURINotAllowed},
		{"https is not a person", statement("https://example.com/bob", "something"), ErrURINotAllowed},
		{"a person is not a command target", command("person://bob", "uptime"), ErrURINotAllowed},
		{"no statement", statement("person://bob", "  "), ErrNoStatement},
		{"no time", func() Evidence { e := statement("person://bob", "x"); e.ObservedAt = nil; return e }(), ErrNoObservedAt},
		// The documented https kinds do not take the new forms.
		{"repository cannot cite a person", shared(SourceRepository, "person://bob"), ErrURINotAllowed},
		{"document cannot cite an azure id", shared(SourceDocument, armID), ErrURINotAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.e.Validate(); !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStatementPerson(t *testing.T) {
	if got := StatementURI("bob"); got != "person://bob" {
		t.Errorf("StatementURI(bob) = %q", got)
	}
	for uri, want := range map[string]ID{"person://bob": "bob", "Person://robert.taylor": "robert.taylor"} {
		if got, err := StatementPerson(uri); err != nil || got != want {
			t.Errorf("StatementPerson(%q) = %q, %v, want %q", uri, got, err, want)
		}
	}
	for _, uri := range []string{"", "person://", "person://bob/x", "person://bob carol", "https://bob", "bob"} {
		if _, err := StatementPerson(uri); !errors.Is(err, ErrURINotAllowed) {
			t.Errorf("StatementPerson(%q) = %v, want ErrURINotAllowed", uri, err)
		}
	}
}

// A command observation or statement with no revision takes the time it
// ran or was said, so each run is its own record and its own root.
func TestEvidenceWithDefaults(t *testing.T) {
	at := time.Date(2026, 10, 21, 9, 30, 0, 500, time.FixedZone("CEST", 7200))
	for _, kind := range []SourceKind{SourceCommand, SourceStatement} {
		e := Evidence{SourceKind: kind, ObservedAt: &at}.WithDefaults()
		if want := "2026-10-21T07:30:00.0000005Z"; e.SourceRevision != want {
			t.Errorf("%s revision = %q, want %q", kind, e.SourceRevision, want)
		}
		given := Evidence{SourceKind: kind, ObservedAt: &at, SourceRevision: "run-7"}.WithDefaults()
		if given.SourceRevision != "run-7" {
			t.Errorf("%s keeps a given revision: %q", kind, given.SourceRevision)
		}
	}
	if e := (Evidence{SourceKind: SourceTicket, ObservedAt: &at}).WithDefaults(); e.SourceRevision != "" {
		t.Errorf("a ticket's revision is never defaulted: %q", e.SourceRevision)
	}
	if e := (Evidence{SourceKind: SourceCommand}).WithDefaults(); e.SourceRevision != "" {
		t.Errorf("no time, no default: %q", e.SourceRevision)
	}
}

// Excerpts, commands and URIs that look like they hold a secret are refused
// with what kind of secret it looks like, never the value.
func TestEvidenceRefusesSecrets(t *testing.T) {
	tests := []struct {
		name, field, value, what string
	}{
		{"connection string password", "excerpt", "Server=tcp:sql-import.database.windows.net;Database=importdb;User ID=app;Password=Sup3rS3cret!;", "password"},
		{"pwd in a connection string", "excerpt", "Driver={ODBC};Server=x;Uid=app;Pwd=hunter22;", "password"},
		{"json password", "excerpt", `{"user": "app", "password": "Sup3rS3cret!"}`, "password"},
		{"url password", "excerpt", "postgres://app:Sup3rS3cret@db.contoso.net:5432/importdb", "password in a URL"},
		{"bearer token", "excerpt", "Authorization: Bearer abcdefghijklmnop0123456789.qrstuvwxyz", "bearer token"},
		{"jwt", "excerpt", "token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", "token"},
		{"storage account key", "excerpt", "DefaultEndpointsProtocol=https;AccountName=stimport;AccountKey=dGhpcyBpcyBub3QgYSByZWFsIGtleSBidXQgaXQgaXMgbG9uZw==;", "key"},
		{"shared access key", "excerpt", "Endpoint=sb://ns.servicebus.windows.net/;SharedAccessKeyName=send;SharedAccessKey=aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789+/=", "key"},
		{"client secret", "excerpt", `"clientSecret": "Abc8Q~xyzXYZ0123456789abcdefgh"`, "secret"},
		{"api key", "excerpt", "x-api-key: 9f8e7d6c5b4a39281706f5e4d3c2b1a0", "key"},
		{"sas signature", "excerpt", "https://stimport.blob.core.windows.net/c/b?sv=2022-11-02&sig=AbCdEfGhIjKlMnOpQrStUvWxYz0123%2B%2F%3D", "key"},
		{"private key", "excerpt", "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...", "private key"},
		{"aws access key", "excerpt", "aws_access_key_id = AKIAIOSFODNN7EXAMPLE", "key"},
		{"github token", "excerpt", "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789", "token"},
		{"secret in the command", "locator", "az sql db show --connection-string 'Server=x;Password=Sup3rS3cret!'", "password"},
		{"password in the uri", "uri", "https://app:Sup3rS3cret@dev.azure.com/org/p/_workitems/edit/1", "password in a URL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := command(armID, "az webapp show")
			if tt.field == "uri" {
				e = shared(SourceTicket, tt.value)
			}
			switch tt.field {
			case "excerpt":
				e.Excerpt = tt.value
			case "locator":
				e.SourceLocator = tt.value
			}
			err := e.Validate()
			if !errors.Is(err, ErrLooksLikeSecret) {
				t.Fatalf("Validate() = %v, want ErrLooksLikeSecret", err)
			}
			msg := err.Error()
			if !strings.Contains(msg, tt.field) || !strings.Contains(msg, tt.what) {
				t.Errorf("message %q should name the field %q and say it looks like a %s", msg, tt.field, tt.what)
			}
			for _, secret := range []string{"Sup3rS3cret", "hunter22", "AKIAIOSFODNN7EXAMPLE", "ghp_abc", "eyJhbGci"} {
				if strings.Contains(msg, secret) {
					t.Errorf("message %q repeats the secret", msg)
				}
			}
		})
	}
}

// Scrubbed values and ordinary output are not secrets.
func TestEvidenceAcceptsScrubbedValues(t *testing.T) {
	for _, excerpt := range []string{
		"Server=tcp:sql-import.database.windows.net;Database=importdb;User ID=app;Password=***;",
		"Server=x;Password=<redacted>;",
		"Server=x;Password=[REDACTED];",
		"Server=x;Password=${SQL_PASSWORD};",
		"Server=x;Password=;",
		"postgres://app:***@db.contoso.net/importdb",
		"postgres://app@db.contoso.net/importdb",
		"Authorization: Bearer <token>",
		`"tokenType": "Bearer", "expiresOn": "2026-10-21"`,
		`"secretName": "import-tool-callback-signing-key"`,
		`"keyVaultSecretId": "https://kv-import.vault.azure.net/secrets/callback"`,
		`"provisioningState": "Succeeded", "httpsOnly": true`,
		"AccountKey=***",
		"commit 3f2a9c1d4e5b6a7980f1e2d3c4b5a69788776655",
	} {
		e := command(armID, "az webapp show")
		e.Excerpt = excerpt
		if err := e.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", excerpt, err)
		}
	}
}
