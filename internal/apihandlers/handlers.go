package apihandlers

import (
	"context"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/contribute"
	"github.com/rztaylor/froodbase/internal/mcpserver"
	"github.com/rztaylor/froodbase/internal/retrieval"
	"github.com/rztaylor/froodbase/internal/store"
	"github.com/rztaylor/froodbase/internal/wire"
)

// New returns the handlers for every tool froodbase-api serves. client
// names this service and version in contribution records.
func New(pool *store.Pool, areas *Areas, client string) mcpserver.Handlers {
	contrib := contribute.New(pool, areas, client)
	reads := retrieval.New(pool, areas)
	return mcpserver.Handlers{
		Whoami: func(ctx context.Context) (mcpserver.WhoamiOutput, error) {
			ac := authz.FromContext(ctx)
			if ac == nil {
				return mcpserver.WhoamiOutput{}, authz.ErrNoContext
			}
			return mcpserver.WhoamiFromContext(ac), nil
		},
		Contribute: func(ctx context.Context, in wire.ContributeInput) (wire.ContributeOutput, error) {
			return contrib.Contribute(ctx, authz.FromContext(ctx), in)
		},
		GetRecord: func(ctx context.Context, in wire.GetRecordInput) (wire.Record, error) {
			return reads.GetRecord(ctx, authz.FromContext(ctx), in)
		},
		GetHistory: func(ctx context.Context, in wire.GetHistoryInput) (wire.HistoryOutput, error) {
			return reads.GetHistory(ctx, authz.FromContext(ctx), in)
		},
		Search: func(ctx context.Context, in wire.SearchInput) (wire.SearchOutput, error) {
			return reads.Search(ctx, authz.FromContext(ctx), in)
		},
	}
}
