package apihandlers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/rztaylor/froodbase/internal/contribute"
	"github.com/rztaylor/froodbase/internal/domain"
	"github.com/rztaylor/froodbase/internal/store"
)

// SystemUser is the actor recorded for writes the service makes on its own
// behalf, such as creating areas at start-up.
const SystemUser = "froodbase-api"

// Areas is the knowledge area directory, backed by the store with a cache
// that refreshes on a miss. It implements contribute.Areas.
type Areas struct {
	pool  *store.Pool
	mu    sync.RWMutex
	byKey map[string]domain.ID
	byID  map[domain.ID]string
}

// NewAreas builds an empty directory.
func NewAreas(pool *store.Pool) *Areas {
	return &Areas{pool: pool, byKey: map[string]domain.ID{}, byID: map[domain.ID]string{}}
}

// Ensure creates an area for every key that has none, with the key as its
// name. It is the local MVP's stand-in for an admin tool.
func (a *Areas) Ensure(ctx context.Context, keys []string) ([]string, error) {
	var created []string
	for _, key := range keys {
		if _, err := a.AreaID(ctx, key); err == nil {
			continue
		} else if !errors.Is(err, contribute.ErrUnknownArea) {
			return created, err
		}
		area := domain.Area{ID: domain.ID(uuid.NewString()), Key: key, Name: key}
		if err := a.pool.CreateArea(ctx, store.Caller{UserID: SystemUser}, area); err != nil {
			return created, fmt.Errorf("creating area %q: %w", key, err)
		}
		a.remember(key, area.ID)
		created = append(created, key)
	}
	return created, nil
}

// AreaID returns the id of the area with key, or contribute.ErrUnknownArea.
func (a *Areas) AreaID(ctx context.Context, key string) (domain.ID, error) {
	a.mu.RLock()
	id, ok := a.byKey[key]
	a.mu.RUnlock()
	if ok {
		return id, nil
	}
	ids, err := a.pool.AreaIDs(ctx, []string{key})
	if err != nil {
		return "", err
	}
	s, ok := ids[key]
	if !ok {
		return "", fmt.Errorf("%w: %q", contribute.ErrUnknownArea, key)
	}
	a.remember(key, domain.ID(s))
	return domain.ID(s), nil
}

// AreaKey returns the key of the area with id. Ids are only ever learnt
// through keys, so an unknown id is an error rather than a lookup.
func (a *Areas) AreaKey(_ context.Context, id domain.ID) (string, error) {
	a.mu.RLock()
	key, ok := a.byID[id]
	a.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("%w: id %s", contribute.ErrUnknownArea, id)
	}
	return key, nil
}

func (a *Areas) remember(key string, id domain.ID) {
	a.mu.Lock()
	a.byKey[key] = id
	a.byID[id] = key
	a.mu.Unlock()
}
