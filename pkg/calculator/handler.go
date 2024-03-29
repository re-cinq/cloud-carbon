package calculator

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"gopkg.in/yaml.v2"

	"github.com/re-cinq/aether/pkg/bus"
	"github.com/re-cinq/aether/pkg/config"
	"github.com/re-cinq/aether/pkg/log"
	v1 "github.com/re-cinq/aether/pkg/types/v1"
	factors "github.com/re-cinq/aether/pkg/types/v1/factors"
	data "github.com/re-cinq/emissions-data/pkg/types/v2"
)

var awsInstances map[string]data.Instance

// AWS, GCP and Azure have increased their server lifespan to 6 years (2024)
// https://sustainability.aboutamazon.com/products-services/the-cloud?energyType=true
// https://www.theregister.com/2024/01/31/alphabet_q4_2023/
// https://www.theregister.com/2022/08/02/microsoft_server_life_extension/
const serverLifespan = 6

// CalculatorHandler is used to handle events when metrics have been collected
type CalculatorHandler struct {
	Bus    *bus.Bus
	logger *slog.Logger
}

// NewHandler returns a new configuered instance of CalculatorHandler
// as well as setups the factor datasets
func NewHandler(ctx context.Context, b *bus.Bus) *CalculatorHandler {
	logger := log.FromContext(ctx)

	err := factors.CloneAndUpdateFactorsData()
	if err != nil {
		logger.Error("error with emissions repo", "error", err)
		return nil
	}

	awsInstances, err = getProviderEC2EmissionFactors(v1.AWS)
	if err != nil {
		logger.Error("unable to get v2 Emission Factors, falling back to v1", "error", err)
	}

	return &CalculatorHandler{
		Bus:    b,
		logger: logger,
	}
}

// Stop is used to fulfill the EventHandler interface and all clean up
// functionality should be run in here
func (c *CalculatorHandler) Stop(ctx context.Context) {}

// Handle is used to fulfill the EventHandler interface and recives an event
// when handler is subscribed to it. Currently only handles v1.MetricsCollectedEvent
func (c *CalculatorHandler) Handle(ctx context.Context, e *bus.Event) {
	switch e.Type {
	case v1.MetricsCollectedEvent:
		c.handleEvent(e)
	default:
		return
	}
}

func getProviderEC2EmissionFactors(provider v1.Provider) (map[string]data.Instance, error) {
	url := "https://raw.githubusercontent.com/re-cinq/emissions-data/main/data/v2/%s-instances.yaml"
	yamlURL := fmt.Sprintf(url, provider)
	resp, err := http.Get(yamlURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	decoder := yaml.NewDecoder(resp.Body)
	err = decoder.Decode(&awsInstances)
	if err != nil {
		return nil, err
	}

	return awsInstances, nil
}

// handleEvent is the business logic for handeling a v1.MetricsCollectedEvent
// and runs the emissions calculations on the metrics that where received
func (c *CalculatorHandler) handleEvent(e *bus.Event) {
	interval := config.AppConfig().ProvidersConfig.Interval

	instance, ok := e.Data.(v1.Instance)
	if !ok {
		c.logger.Error("EmissionCalculator got an unknown event", "event", e)
		return
	}

	// Gets PUE, grid data, and machine specs
	emFactors, err := factors.GetProviderEmissionFactors(
		instance.Provider,
		factors.DataPath,
	)
	if err != nil {
		c.logger.Error("error getting emission factors", "error", err)
		return
	}

	gridCO2eTons, ok := emFactors.Coefficient[instance.Region]
	if !ok {
		c.logger.Error("region does not exist in factors for provider", "region", instance.Region, "provider", "gcp")
		return
	}

	// TODO: hotfix until updated in emissions data
	// convert gridCO2e from metric tonnes to grams
	gridCO2e := gridCO2eTons * (1000 * 1000)

	params := parameters{
		gridCO2e: gridCO2e,
		pue:      emFactors.AveragePUE,
	}

	specs, ok := emFactors.Embodied[instance.Kind]
	if !ok {
		c.logger.Error("failed finding instance in factor data", "instance", instance.Name, "kind", instance.Kind)
		return
	}

	if d, ok := awsInstances[instance.Kind]; ok {
		params.wattage = d.PkgWatt
		params.vCPU = float64(d.VCPU)
		params.embodiedFactor = d.EmbodiedHourlyGCO2e
	} else {
		params.wattage = []data.Wattage{
			{
				Percentage: 0,
				Wattage:    specs.MinWatts,
			},
			{
				Percentage: 100,
				Wattage:    specs.MaxWatts,
			},
		}
		params.embodiedFactor = hourlyEmbodiedEmissions(&specs)
	}

	// calculate and set the operational emissions for each
	// metric type (CPU, Memory, Storage, and networking)
	metrics := instance.Metrics
	for _, v := range metrics {
		params.metric = &v
		opEm, err := operationalEmissions(log.WithContext(context.Background(), c.logger), interval, &params)
		if err != nil {
			c.logger.Error("failed calculating operational emissions", "type", v.Name, "error", err)
			continue
		}
		params.metric.Emissions = v1.NewResourceEmission(opEm, v1.GCO2eqkWh)
		// update the instance metrics
		metrics.Upsert(params.metric)
	}

	instance.EmbodiedEmissions = v1.NewResourceEmission(
		embodiedEmissions(interval, params.embodiedFactor),
		v1.GCO2eqkWh,
	)

	// We publish the interface on the bus once its been calculated
	if err := c.Bus.Publish(&bus.Event{
		Type: v1.EmissionsCalculatedEvent,
		Data: instance,
	}); err != nil {
		c.logger.Error("failed publishing instance after calculation", "instance", instance.Name, "error", err)
	}
}

func hourlyEmbodiedEmissions(e *factors.Embodied) float64 {
	// we fall back on the specs from the previous dataset
	// and convert it into a hourly factor
	// this is based on CCF's calculation:
	//
	// M = TE * (TR/EL) * (RR/TR)
	//
	// TE = Total Embodied Emissions
	// TR = Time Reserved (in years)
	// EL = Expected Lifespan
	// RR = Resources Reserved
	// TR = Total Resources, the total number of resources available.
	return e.TotalEmbodiedKiloWattCO2e *
		// 1 hour normalized to a year
		((1.0 / 24.0 / 365.0) / serverLifespan) *
		// amount of vCPUS for instance versus total vCPUS for platform
		(e.VCPU / e.TotalVCPU)
}
