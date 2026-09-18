package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"

	"github.com/jusoaresg/gorgon/config"
	"github.com/jusoaresg/gorgon/external/prowlarr/schema"
	"github.com/jusoaresg/gorgon/pkg/services"
	"golang.org/x/time/rate"
)

type ErrProwlarrHostPortNotSet interface {
	Error() string
	IsProwlarrConfigWarn() bool
}
type ProwlarrHostPortNotSet struct{}

func (m *ProwlarrHostPortNotSet) Error() string {
	return "Prowlar Host or Port not set"
}

func (m *ProwlarrHostPortNotSet) IsProwlarrConfigWarn() bool {
	return true
}

var (
	interactiveLimiter   = rate.NewLimiter(5, 10)
	interactiveSemaphore = make(chan struct{}, 5)
)

type ProwlarrSearchService struct {
	ApiKey     string
	APIService services.APIService
	Logger     *slog.Logger
	limiter    *rate.Limiter
	semaphore  chan struct{}
}

func NewProwlarrSearchService(logger *slog.Logger) (*ProwlarrSearchService, error) {
	return newProwlarrSearchService(logger, rate.Limit(5), 10, 5)
}

func NewInteractiveProwlarrSearchService(logger *slog.Logger) (*ProwlarrSearchService, error) {
	configFile, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	host := configFile.ProwlarrHost
	port := configFile.ProwlarrPort

	if host == "" || port == "" {
		return nil, &ProwlarrHostPortNotSet{}
	}

	return &ProwlarrSearchService{
		ApiKey:     configFile.ProwlarrApiKey,
		Logger:     logger,
		APIService: *services.NewAPIService(fmt.Sprintf("%s:%s", host, port), logger),
		limiter:    interactiveLimiter,
		semaphore:  interactiveSemaphore,
	}, nil
}

func newProwlarrSearchService(logger *slog.Logger, limit rate.Limit, burst int, semSize int) (*ProwlarrSearchService, error) {
	configFile, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	host := configFile.ProwlarrHost
	port := configFile.ProwlarrPort

	if host == "" || port == "" {
		return nil, &ProwlarrHostPortNotSet{}
	}

	return &ProwlarrSearchService{
		ApiKey:     configFile.ProwlarrApiKey,
		Logger:     logger,
		APIService: *services.NewAPIService(fmt.Sprintf("%s:%s", host, port), logger),
		limiter:    rate.NewLimiter(limit, burst),
		semaphore:  make(chan struct{}, semSize),
	}, nil
}

func (p *ProwlarrSearchService) Name() string {
	return "ProwlarrSearch"
}

func (p *ProwlarrSearchService) waitAndLock(ctx context.Context) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	p.semaphore <- struct{}{}
	return nil
}

func (p *ProwlarrSearchService) unlock() {
	<-p.semaphore
}

func (p *ProwlarrSearchService) CheckConnection() error {
	var resp struct {
		Status string `json:"status"`
	}
	err := p.APIService.Get("/ping", &resp)
	if err != nil {
		return err
	}
	return nil
}

func (p *ProwlarrSearchService) Search(request *schema.SearchRequest, model *[]schema.SearchResponse, indexerIds ...int) error {
	if err := p.waitAndLock(context.Background()); err != nil {
		return err
	}
	defer p.unlock()

	params := url.Values{}
	params.Set("query", request.Query)
	params.Set("apikey", p.ApiKey)

	for _, indexer := range indexerIds {
		params.Add("indexerIds", strconv.Itoa(indexer))
	}

	return p.APIService.Get(fmt.Sprintf("/api/v1/search?%s", params.Encode()), &model)
}

func (p *ProwlarrSearchService) SearchByType(request *schema.SearchByTypeRequest, model *[]schema.SearchResponse, indexerIds ...int) error {
	if err := p.waitAndLock(context.Background()); err != nil {
		return err
	}
	defer p.unlock()

	params := url.Values{}
	params.Set("query", request.Query)
	params.Set("type", request.Type)
	params.Set("apikey", p.ApiKey)
	for _, id := range indexerIds {
		params.Add("indexerIds", strconv.Itoa(id))
	}
	return p.APIService.Get(fmt.Sprintf("/api/v1/search?%s", params.Encode()), &model)
}
