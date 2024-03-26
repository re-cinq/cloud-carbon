package amazon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/patrickmn/go-cache"
	"github.com/re-cinq/aether/pkg/providers/util"
	v1 "github.com/re-cinq/aether/pkg/types/v1"
)

// Helper service to get CloudWatch data
type cloudWatchClient struct {
	client *cloudwatch.Client
}

// New cloudwatch client instance
func NewCloudWatchClient(ctx context.Context, cfg *aws.Config) *cloudWatchClient {
	emptyOptions := func(o *cloudwatch.Options) {}

	// Init the Cloudwatch client
	client := cloudwatch.NewFromConfig(*cfg, emptyOptions)

	// Make sure the initialisation was successful
	if client == nil {
		slog.Error("failed to create AWS CloudWatch client")
		os.Exit(1)
	}

	// Return the cloudwatch service client
	return &cloudWatchClient{
		client: client,
	}
}

// Get the resource consumption of an ec2 instance
func (e *cloudWatchClient) GetEC2Metrics(ca *cache.Cache, region string, interval time.Duration) ([]v1.Instance, error) {
	instances := []v1.Instance{}
	local := make(map[string]*v1.Instance)

	end := time.Now().UTC()
	start := end.Add(-interval)

	// Get the cpu consumption for all the instances in the region
	cpuMetrics, err := e.getEC2CPU(region, start, end, interval)
	if err != nil {
		return instances, err
	}

	if len(cpuMetrics) == 0 {
		return instances, fmt.Errorf("no cpu metrics collected from CloudWatch")
	}

	// TODO: Will need to iterate cpuMetrics and memMetrics
	for i := range cpuMetrics {
		// to avoid Implicit memory aliasing in for loop
		metric := cpuMetrics[i]

		instanceID, ok := metric.Labels["instanceID"]
		if !ok {
			slog.Error("metric does not have an instanceID", "metric", metric)
			continue
		}

		// load the instance metadata from the cache, because the query does not give us instance info
		cachedInstance, exists := ca.Get(util.CacheKey(region, ec2Service, instanceID))
		if cachedInstance == nil || !exists {
			slog.Warn("instance id  is not present in the metadata, temporarily skipping collecting metrics", "id", instanceID)
			continue
		}

		meta := cachedInstance.(*v1.Resource)

		// update local instance metadata map
		s, exists := local[instanceID]
		if !exists {
			// Then create a new local instance from cached
			s = &v1.Instance{
				Name:     instanceID,
				Provider: provider,
				Service:  ec2Service, // EC2
				Kind:     meta.Kind,
				Region:   region,
			}
		}
		s.Labels.Add("Name", meta.Name)
		metric.UnitAmount = float64(meta.VCPUCount)
		s.Metrics.Upsert(&metric)

		local[instanceID] = s
		instances = append(instances, *s)
	}

	return instances, nil
}

// Get the CPU resource consumption of an ec2 instance
func (e *cloudWatchClient) getEC2CPU(region string, start, end time.Time, interval time.Duration) ([]v1.Metric, error) {
	// Override the region
	withRegion := func(o *cloudwatch.Options) {
		o.Region = region
	}

	period := int32(interval.Seconds())
	// validate the casting from float64 to int32
	if float64(period) != interval.Seconds() {
		return nil, fmt.Errorf("error casting %+v to int32", interval.Seconds())
	}

	// Make the call to get the CPU metrics
	output, err := e.client.GetMetricData(context.TODO(), &cloudwatch.GetMetricDataInput{
		StartTime: &start,
		EndTime:   &end,
		MetricDataQueries: []types.MetricDataQuery{
			{
				Id:         aws.String(v1.CPU.String()),
				Expression: aws.String(`SELECT AVG(CPUUtilization) FROM "AWS/EC2" GROUP BY InstanceId`),
				Period:     aws.Int32(period),
			},
		},
	}, withRegion)
	if err != nil {
		return nil, err
	}

	// Collector
	var cpuMetrics []v1.Metric

	// Loop through the result and build the intermediate awsMetric model
	for _, metric := range output.MetricDataResults {
		instanceID := aws.ToString(metric.Label)
		if instanceID == "Other" {
			return nil, errors.New("error bad query passed to GetMetricData - instanceID not found in label")
		}

		if len(metric.Values) > 0 {
			cpu := v1.NewMetric(v1.CPU.String())
			cpu.Unit = v1.VCPU
			cpu.Usage = metric.Values[0]
			cpu.ResourceType = v1.CPU
			cpu.Labels = v1.Labels{
				"instanceID": instanceID,
			}
			cpuMetrics = append(cpuMetrics, *cpu)
		}
	}

	return cpuMetrics, nil
}
