package paths

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var (
	pathsCache        *cache.Cache
	pathsCacheEnabled = false
)

// InitPathsCache inits paths for the cache.
func InitPathsCache(pathsCacheTTL time.Duration, pathsCachePurgeInterval time.Duration) {
	pathsCache = cache.New(pathsCacheTTL, pathsCachePurgeInterval)
	pathsCacheEnabled = true
}
