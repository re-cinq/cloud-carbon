// Contains a set of method for getting EC2 information
package amazon

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/patrickmn/go-cache"
	"github.com/re-cinq/aether/pkg/providers/util"
	v1 "github.com/re-cinq/aether/pkg/types/v1"
)

// Helper service to get EC2 data
type ec2Client struct {
	client *ec2.Client
}

// New instance
func NewEC2Client(cfg *aws.Config) *ec2Client {
	emptyOptions := func(o *ec2.Options) {}

	// Init the EC2 client
	client := ec2.NewFromConfig(*cfg, emptyOptions)

	// Make sure the initialisation was successful
	if client == nil {
		slog.Error("failed to create AWS EC2 client")
		return nil
	}

	// Return the ec2 service
	return &ec2Client{
		client: client,
	}
}

// refresh stores all the instances for a specific region in cache
func (e *ec2Client) Refresh(ctx context.Context, ca *cache.Cache, region string) error {
	// Override the region
	withRegion := func(o *ec2.Options) {
		o.Region = region
	}

	// First request
	output, err := e.client.DescribeInstances(ctx, buildListPaginationRequest(nil), withRegion)
	if err != nil || output == nil {
		return fmt.Errorf("failed to retrieve ec2 instances from region: %s: %s", region, err)
	}

	// Collect all the responses for all the pages
	instances := []ec2.DescribeInstancesOutput{*output}

	for output.NextToken != nil {
		output, err = e.client.DescribeInstances(ctx, buildListPaginationRequest(output.NextToken), withRegion)
		if err != nil || output == nil {
			return fmt.Errorf("failed to retrieve ec2 instances %s", err)
		}

		instances = append(instances, *output)
	}

	for _, reservation := range output.Reservations {
		for index := range reservation.Instances {
			instance := reservation.Instances[index]

			id := aws.ToString(instance.InstanceId)
			ca.Set(util.CacheKey(region, ec2Service, id),
				&v1.Resource{
					ID:          id,
					Name:        getInstanceTag(instance.Tags, "Name"),
					Service:     ec2Service,
					Region:      region,
					Kind:        string(instance.InstanceType),
					Lifecycle:   string(instance.InstanceLifecycle),
					VCPUCount:   int(aws.ToInt32(instance.CpuOptions.CoreCount) * aws.ToInt32(instance.CpuOptions.ThreadsPerCore)),
					LastUpdated: time.Now().UTC(),
				},
				cache.DefaultExpiration,
			)
		}
	}
	return nil
}

func getInstanceTag(tags []types.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

func buildListPaginationRequest(nextToken *string) *ec2.DescribeInstancesInput {
	return &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []string{"running", "pending"},
			},
		},
		MaxResults: aws.Int32(50),
		NextToken:  nextToken,
	}
}
