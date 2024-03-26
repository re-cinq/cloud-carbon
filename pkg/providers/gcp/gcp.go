package gcp

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	"cloud.google.com/go/compute/apiv1/computepb"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	cache "github.com/patrickmn/go-cache"
	"github.com/re-cinq/aether/pkg/config"
	"github.com/re-cinq/aether/pkg/log"
	"github.com/re-cinq/aether/pkg/providers/util"
	v1 "github.com/re-cinq/aether/pkg/types/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Client is the structure used as the provider for Google Cloud Platform
type Client struct {
	// GCP Clients
	monitoring *monitoring.QueryClient
	instances  *compute.InstancesClient

	// Caching mechanism
	cache *cache.Cache
}

type options func(*Client)

// New returns a new instance of the GCP provider as well as a function to
// cleanup connections once done
func New(
	ctx context.Context,
	account *config.Account,
	opts ...options,
) (c *Client, teardown func(), err error) {
	// set any defaults here
	c = &Client{
		// TODO do we want to expire cache?
		cache: cache.New(3600*time.Minute, 3600*time.Minute),
	}

	var clientOptions []option.ClientOption

	if account.Credentials.IsPresent() {
		credentialFile := account.Credentials.FilePaths[0]
		clientOptions = append(
			clientOptions,
			option.WithCredentialsFile(credentialFile),
		)
	}

	// overwrite any options
	for _, opt := range opts {
		opt(c)
	}

	// This allows overwriting the default monitoring client
	// google by default trys to authenticate when initilizing
	// a client therefore if we put this before running the options
	// it would try authenticate against google regardless of overwriting the
	// client
	if c.monitoring == nil {
		mc, err := monitoring.NewQueryClient(ctx, clientOptions...)
		if err != nil {
			return nil, func() {}, err
		}
		c.monitoring = mc
	}

	// This allows overwriting the default instances client
	if c.instances == nil {
		ic, err := compute.NewInstancesRESTClient(ctx, clientOptions...)
		if err != nil {
			return nil, func() {}, err
		}
		c.instances = ic
	}

	// teardown is used to close relevant connections
	// and cleanup
	teardown = func() {
		c.monitoring.Close()
		c.instances.Close()
	}

	return c, teardown, nil
}

// GetMetricsForInstances retrieves all the metrics for a given instance
func (c *Client) GetMetricsForInstances(
	ctx context.Context,
	project, window string,
) ([]v1.Instance, error) {
	var instances []v1.Instance

	cpumetrics, err := c.instanceCPUMetrics(
		// TODO these parameters can be cleaned up
		ctx, project, fmt.Sprintf(CPUQuery, project, window, window),
	)
	if err != nil {
		return instances, err
	}

	memmetrics, err := c.instanceMemoryMetrics(
		ctx, project, fmt.Sprintf(MEMQuery, project, window, window),
	)
	if err != nil {
		return instances, err
	}

	// we use a lookup to add different metrics to the same instance
	lookup := make(map[string]*v1.Instance)

	// TODO there seems to be duplicated logic here
	// Why not create instance whuile collecting metric instead of handeling
	// it in two steps
	for _, m := range append(cpumetrics, memmetrics...) {
		metric := *m

		meta, err := getMetadata(&metric)
		if err != nil {
			continue
		}

		// Load the cache
		// TODO make this more explicit, im not sure why this
		// is needed as we dont use the cache anywhere
		// I think this is removed, and is used when the metric data doesn't have
		// the complete resource/instance information.
		cachedInstance, ok := c.cache.Get(util.CacheKey(meta.zone, service, meta.name))
		if cachedInstance == nil && !ok {
			continue
		}

		i, ok := lookup[meta.id]
		if !ok {
			i = v1.NewInstance(meta.id, provider)
			i.Service = service
		}

		i.Kind = meta.machineType
		i.Region = meta.region
		i.Zone = meta.zone
		i.Metrics.Upsert(&metric)

		lookup[meta.id] = i
	}

	// create list of instances
	// TODO: this seems repetitive
	for _, v := range lookup {
		instances = append(instances, *v)
	}

	return instances, nil
}

type metadata struct {
	zone, region, name, id, machineType string
}

func getMetadata(m *v1.Metric) (*metadata, error) {
	zone, ok := m.Labels["zone"]
	if !ok {
		return &metadata{}, errors.New("zone not found")
	}

	region, ok := m.Labels["region"]
	if !ok {
		return &metadata{}, errors.New("region not found")
	}

	name, ok := m.Labels["name"]
	if !ok {
		return &metadata{}, errors.New("instance name not found")
	}

	id, ok := m.Labels["id"]
	if !ok {
		return &metadata{}, errors.New("instance id not found")
	}

	machineType, ok := m.Labels["machine_type"]
	if !ok {
		return &metadata{}, errors.New("machine type not found")
	}
	return &metadata{
		zone:        zone,
		region:      region,
		name:        name,
		id:          id,
		machineType: machineType,
	}, nil
}

// Refresh fetches all the Instances
// for a project and stores metadata in order to help with
// metric collections
func (c *Client) Refresh(ctx context.Context, project string) {
	logger := log.FromContext(ctx)

	iter := c.instances.AggregatedList(
		ctx,
		&computepb.AggregatedListInstancesRequest{
			Project: project,
		},
	)

	for {
		resp, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			logger.Error("failed processesing GCE instance", "error", err)
			return
		}

		for _, instance := range resp.Value.Instances {
			zone, err := getValueFromURL(instance.GetZone())
			if err != nil {
				logger.Error("failed to get zone from url")
			}
			instanceID := strconv.FormatUint(instance.GetId(), 10)
			name := instance.GetName()

			if zone == "" {
				continue
			}

			if instance.GetStatus() == "TERMINATED" {
				// delete the entry from the cache
				c.cache.Delete(util.CacheKey(zone, service, name))
				continue
			}

			if instance.GetStatus() == "RUNNING" {
				kind, err := getValueFromURL(instance.GetMachineType())
				if err != nil {
					logger.Error("failed to get instance type from url")
				}
				c.cache.Set(util.CacheKey(zone, service, name), v1.Instance{
					Name:    name,
					Zone:    zone,
					Service: service,
					Kind:    kind,
					Labels: v1.Labels{
						"Lifecycle": instance.GetScheduling().GetProvisioningModel(),
						"ID":        instanceID,
					},
				}, cache.DefaultExpiration)
			}
		}
	}
}

// getValueFromURL returns the last element in the url Path
// example:
// input: https://www.googleapis.com/.../machineTypes/e2-micro
// output: e2-micro
func getValueFromURL(u string) (string, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}

	return path.Base(parsed.Path), nil
}
