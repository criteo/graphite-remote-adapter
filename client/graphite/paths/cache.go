package paths

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var (
	pathsCache        *cache.Cache
	pathsCacheEnabled = false
)

func InitPathsCache(pathsCacheTTL time.Duration, pathsCachePurgeInterval time.Duration) {
	pathsCache = cache.New(pathsCacheTTL, pathsCachePurgeInterval)
	pathsCacheEnabled = true
}
