package service

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/pkg/services"
)

var (
	indexerCacheMu     sync.RWMutex
	indexerCache       []schema.IndexerResponse
	indexerCacheExpiry time.Time
)

type ProwlarrIndexerService struct {
	ApiKey     string
	APIService services.APIService
	Logger     *slog.Logger
}

func NewProwlarrIndexerService(logger *slog.Logger) *ProwlarrIndexerService {
	configFile, err := config.LoadConfig()
	if err != nil {
		panic("Error while creting Prowlarr Indexer Service")
	}

	prowlarrHost := configFile.ProwlarrHost
	prowlarrPort := configFile.ProwlarrPort
	return &ProwlarrIndexerService{
		ApiKey:     configFile.ProwlarrApiKey,
		Logger:     logger,
		APIService: *services.NewAPIService(fmt.Sprintf("%s:%s", prowlarrHost, prowlarrPort), logger),
	}
}

func (p *ProwlarrIndexerService) GetIndexers(model *[]schema.IndexerResponse) error {
	indexerCacheMu.RLock()
	if time.Now().Before(indexerCacheExpiry) && len(indexerCache) > 0 {
		*model = make([]schema.IndexerResponse, len(indexerCache))
		copy(*model, indexerCache)
		indexerCacheMu.RUnlock()
		return nil
	}
	indexerCacheMu.RUnlock()

	indexerCacheMu.Lock()
	defer indexerCacheMu.Unlock()

	// Double check after acquiring write lock
	if time.Now().Before(indexerCacheExpiry) && len(indexerCache) > 0 {
		*model = make([]schema.IndexerResponse, len(indexerCache))
		copy(*model, indexerCache)
		return nil
	}

	var fetched []schema.IndexerResponse
	if err := p.APIService.Get(fmt.Sprintf("/api/v1/indexer?apikey=%s", p.ApiKey), &fetched); err != nil {
		return err
	}

	indexerCache = fetched
	indexerCacheExpiry = time.Now().Add(2 * time.Minute)
	*model = fetched
	return nil
}

func (p *ProwlarrIndexerService) GetIndexer(id int, model *schema.IndexerResponse) error {
	return p.APIService.Get(fmt.Sprintf("/api/v1/indexer/%d?apikey=%s", id, p.ApiKey), &model)
}
