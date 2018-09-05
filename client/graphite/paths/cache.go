package paths

import (
	"time"

	"github.com/patrickmn/go-cache"
)

var (
	pathsCache        *cache.Cache
	pathsCacheEnabled = false
)

// InitPathsCache inits cache for the paths.
func InitPathsCache(pathsCacheTTL time.Duration, pathsCachePurgeInterval time.Duration) {
	pathsCache = cache.New(pathsCacheTTL, pathsCachePurgeInterval)
	pathsCacheEnabled = true
}
