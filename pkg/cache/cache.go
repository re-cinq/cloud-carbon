package cache

import (
	"context"
	"fmt"
	"time"

	bc "github.com/allegro/bigcache/v3"
	eko "github.com/eko/gocache/lib/v4/cache"
	store "github.com/eko/gocache/store/bigcache/v4"
	"github.com/re-cinq/aether/pkg/config"
)

func New(ctx context.Context) (*eko.Cache[string], error) {
	//	logger := log.FromContext(ctx)
	config := config.AppConfig().Cache

	switch config.Store {
	case "bigcache":
		return bigcache(ctx, config.Expiry)
	default:
		return nil, fmt.Errorf("error cache not yet supported: %s", config.Store)
	}
}

func bigcache(ctx context.Context, expiry time.Duration) (*eko.Cache[string], error) {
	cli, err := bc.New(ctx, bc.DefaultConfig(expiry))
	if err != nil {
		return nil, err
	}

	return eko.New[string](store.NewBigcache(cli)), nil
}
