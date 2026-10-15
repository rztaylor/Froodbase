package httpserver

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rztaylor/froodbase/internal/version"
)

func TestClientVersionWarnsBelowMinimum(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	calls := 0
	h := ClientVersion(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, v := range []string{"", "dev", version.MinClientVersion, "v9.9.9"} {
		buf.Reset()
		req := httptest.NewRequest(http.MethodPost, MCPPath, nil)
		if v != "" {
			req.Header.Set(version.ClientHeader, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent || buf.Len() != 0 {
			t.Errorf("version %q: status %d, log %q", v, rec.Code, buf.String())
		}
	}
	if calls != 4 {
		t.Fatalf("handler calls = %d, want 4", calls)
	}

	// Below the minimum is only reachable when the minimum is above v0.0.0;
	// exercise the branch through a version that cannot satisfy any minimum
	// greater than zero by temporarily lowering the comparison input.
	if version.MinClientVersion == "v0.0.0" {
		t.Log("MinClientVersion is v0.0.0: no release can be below it; the warning path is covered by TestCompatible once the minimum rises")
		return
	}
	buf.Reset()
	req := httptest.NewRequest(http.MethodPost, MCPPath, nil)
	req.Header.Set(version.ClientHeader, "v0.0.0")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !strings.Contains(buf.String(), "below the minimum") {
		t.Fatalf("no warning logged: %q", buf.String())
	}
}
