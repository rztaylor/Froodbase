package contribute

import (
	"context"
	"errors"

	"github.com/rztaylor/froodbase/internal/authz"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
)

// Areas maps knowledge area keys (as grants name them) to ids and back. The
// API supplies an implementation backed by the store.
type Areas interface {
	AreaID(ctx context.Context, key string) (domain.ID, error)
	AreaKey(ctx context.Context, id domain.ID) (string, error)
}

// CallerFor builds the store caller for an authorisation context: the
// person, the agent acting for them and the ids of every area they hold any
// grant in. Areas without a record are skipped.
func CallerFor(ctx context.Context, areas Areas, ac *authz.Context) (store.Caller, error) {
	c := store.Caller{UserID: ac.UserID, AgentID: ac.AgentID}
	for _, key := range ac.Areas() {
		id, err := areas.AreaID(ctx, key)
		if err != nil {
			if errors.Is(err, ErrUnknownArea) {
				continue
			}
			return store.Caller{}, err
		}
		c.AreaIDs = append(c.AreaIDs, string(id))
	}
	return c, nil
}
