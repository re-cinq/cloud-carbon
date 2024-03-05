package amazon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/patrickmn/go-cache"
	"github.com/re-cinq/cloud-carbon/pkg/providers/util"
	v1 "github.com/re-cinq/cloud-carbon/pkg/types/v1"
	"k8s.io/klog/v2"
)

const cpuExpression = `SELECT AVG(CPUUtilization) FROM "AWS/EC2" GROUP BY InstanceId`
const memExpression = `SELECT AVG(mem_used_percent) FROM SCHEMA(CWAgent, InstanceId) GROUP BY InstanceId`

// Helper service to get CloudWatch data
type cloudWatchClient struct {
	client *cloudwatch.Client
}

// New cloudwatch client instance
func NewCloudWatchClient(cfg *aws.Config) *cloudWatchClient {
	emptyOptions := func(o *cloudwatch.Options) {}

	// Init the Cloudwatch client
	client := cloudwatch.NewFromConfig(*cfg, emptyOptions)

	// Make sure the initialisation was successful
	if client == nil {
		klog.Fatal("failed to create AWS CloudWatch client")
		return nil
	}

	// Return the cloudwatch service client
	return &cloudWatchClient{
		client: client,
	}
}

// runQuery runs the cloudwatch expression to get metric data
func (e *cloudWatchClient) runQuery(ctx context.Context, exp string, region string, interval time.Duration) ([]types.MetricDataResult, error) {
	end := time.Now().UTC()
	start := end.Add(-interval)

	// Override the region
	withRegion := func(o *cloudwatch.Options) {
		o.Region = region
	}

	period := int32(interval.Seconds())
	// validate the casting from float64 to int32
	if float64(period) != interval.Seconds() {
		return nil, fmt.Errorf("error casting %+v to int32", interval.Seconds())
	}

	output, err := e.client.GetMetricData(ctx, &cloudwatch.GetMetricDataInput{
		StartTime: &start,
		EndTime:   &end,
		MetricDataQueries: []types.MetricDataQuery{
			{
				Id:         aws.String(v1.Memory.String()),
				Expression: aws.String(exp),
				Period:     aws.Int32(period),
			},
		},
	}, withRegion)

	return output.MetricDataResults, err
}

// Get the resource consumption of an ec2 instance
func (e *cloudWatchClient) GetEC2Metrics(ca *cache.Cache, region string, interval time.Duration) ([]v1.Instance, error) {
	instances := []v1.Instance{}
	local := make(map[string]*v1.Instance)
	metrics := []v1.Metric{}

	ctx := context.Background()

	// Get the cpu consumption for all the instances in the region
	cpuMetrics, err := e.runQuery(ctx, cpuExpression, region, interval)
	if err != nil {
		return instances, err
	}

	// convert aws ouput to metric type
	for _, metric := range cpuMetrics {
		instanceID := aws.ToString(metric.Label)
		if instanceID == "Other" {
			klog.Warning("error bad query passed to GetMetricData - instanceID not found in label")
			continue
		}

		if len(metric.Values) > 0 {
			m := v1.NewMetric(v1.CPU.String())
			m.SetUsage(metric.Values[0]).SetType(v1.CPU)
			m.SetLabels(map[string]string{
				"instanceID": instanceID,
			})
			metrics = append(metrics, *m)
		}
	}

	// get the memory utilization for all the instances in the region
	memMetrics, err := e.runQuery(ctx, memExpression, region, interval)
	if err != nil {
		return instances, err
	}

	// convert aws ouput to metric type
	for _, metric := range memMetrics {
		instanceID := aws.ToString(metric.Label)
		if instanceID == "Other" {
			klog.Warning("error bad query passed to GetMetricData - instanceID not found in label")
			continue
		}

		if len(metric.Values) > 0 {
			m := v1.NewMetric(v1.Memory.String())
			m.SetUsage(metric.Values[0]).SetType(v1.Memory)
			m.SetLabels(map[string]string{
				"instanceID": instanceID,
			})
			metrics = append(metrics, *m)
		}
	}

	for _, metric := range metrics {
		instanceID, ok := metric.Labels().Get("instanceID")
		if !ok {
			klog.Errorf("error metric doesn't have an instanceID: %+v", metric)
			continue
		}

		// load the instance metadata from the cache, because the query does not give us instance info
		cachedInstance, exists := ca.Get(util.CacheKey(region, ec2Service, instanceID))
		if cachedInstance == nil || !exists {
			klog.Warningf("instance id %s is not present in the metadata, temporarily skipping collecting metrics", instanceID)
			continue
		}

		meta := cachedInstance.(*v1.Resource)

		// update local instance metadata map
		s, exists := local[instanceID]
		if !exists {
			// Then create a new local instance from cached
			s = v1.NewInstance(instanceID, provider)
			s.SetService("EC2")
			s.SetKind(meta.Kind)
			s.SetRegion(region)
		}

		s.AddLabel("Name", meta.Name)
		s.Metrics().Upsert(&metric)

		local[instanceID] = s
		instances = append(instances, *s)
	}

	return instances, nil
}
