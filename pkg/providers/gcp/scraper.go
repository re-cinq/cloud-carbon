package gcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/re-cinq/aether/pkg/bus"
	"github.com/re-cinq/aether/pkg/config"
	"github.com/re-cinq/aether/pkg/log"
	v1 "github.com/re-cinq/aether/pkg/types/v1"
)

// Scraper is used to handle scraping google for various metrics
type Scraper struct {
	// Google Client
	*Client

	// Ticker
	ticker *time.Ticker
	Done   chan bool

	// The Project that Client is setup for
	Project *string

	// Event bus for publishing
	Bus *bus.Bus

	// Teradown functionality
	Shutdown func()

	logger *slog.Logger
}

// SetupScrapers instantiates a slice of instances of the Google Scraper configured
// for use, one per project configured
func SetupScrapers(ctx context.Context, b *bus.Bus) []v1.Scraper {
	cfg, exists := config.AppConfig().Providers[provider]
	logger := log.FromContext(ctx)

	// If the provider is not configured - skip its initialization
	if !exists {
		return nil
	}

	var scrapers []v1.Scraper

	// we instantiate a scraper per project
	for index := range cfg.Accounts {
		account := cfg.Accounts[index]

		ticker := time.NewTicker(config.AppConfig().ProvidersConfig.Interval)

		c, shutdown, err := New(ctx, &account)
		if err != nil {
			return nil
		}

		// this is where we populate the cache
		c.Refresh(ctx, account.Project)

		scrapers = append(scrapers, &Scraper{
			ticker:   ticker,
			Done:     make(chan bool),
			Project:  &account.Project,
			Bus:      b,
			Client:   c,
			Shutdown: shutdown,
			logger:   logger,
		})
	}

	return scrapers
}

// Start runs the scraper at the interval set by the ticker
func (s *Scraper) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-s.Done:
				return
			case <-s.ticker.C:
				s.logger.Info("running scraper for gcp")
				err := s.scrape(ctx)
				if err != nil {
					s.logger.Error("scraping error", "error", err)
				}
			}
		}
	}()

	// we run the scraper once first in order to populate data as quickly as
	// possible
	err := s.scrape(ctx)
	if err != nil {
		s.logger.Error("scraping error", "error", err)
	}
}

// scrape handles updating and fetching the data from google
func (s *Scraper) scrape(ctx context.Context) error {
	if s.Project == nil {
		return errors.New("no project set")
	}

	// we need to repopulate the cahce on every scrape
	// TODO: maybe we dont need cache?
	s.Client.Refresh(ctx, *s.Project)

	interval := config.AppConfig().Interval
	if interval < 5*time.Minute {
		return fmt.Errorf("error interval for GCP needs to be atleast 5m. It is: %+v", interval)
	}

	instances, err := s.Client.GetMetricsForInstances(ctx, *s.Project, interval.String())

	if err != nil {
		return fmt.Errorf("failed getting instances: %v", err)
	}

	for i := range instances {
		e := s.Bus.Publish(&bus.Event{
			Type: v1.MetricsCollectedEvent,
			Data: instances[i],
		})
		if e != nil {
			err = errors.Join(
				err,
				fmt.Errorf("failed to publish for instance %s: %v", instances[i].Name, e),
			)
		}
	}

	return err
}

// Stop is used to gracefully stop the scrapper
func (s *Scraper) Stop(ctx context.Context) {
	s.Done <- true

	s.ticker.Stop()
	s.Shutdown()
}
