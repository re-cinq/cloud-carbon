package calculator

import (
	"testing"
	"time"

	v1 "github.com/re-cinq/cloud-carbon/pkg/types/v1"
	data "github.com/re-cinq/emissions-data/pkg/types/v2"
	"github.com/stretchr/testify/assert"
)

func params() *parameters {
	m := v1.NewMetric("basic")
	m.SetType(v1.CPU).SetUsage(27)
	return &parameters{
		gridCO2e: 7,
		pue:      1.2,
		metric:   m,
		// using t3.micro AWS instance as default
		wattage: []data.Wattage{
			{
				Percentage: 0,
				Wattage:    1.21,
			},
			{
				Percentage: 10,
				Wattage:    3.05,
			},
			{
				Percentage: 50,
				Wattage:    7.16,
			},
			{
				Percentage: 100,
				Wattage:    9.96,
			},
		},
		vCPU:           2,
		embodiedFactor: 1000,
	}
}

func TestCalculateCPU(t *testing.T) {
	type testcase struct {
		name     string
		interval time.Duration // this is nanoseconds
		params   *parameters
		expRes   float64
		hasErr   bool
		expErr   string
	}

	for _, test := range []*testcase{
		func() *testcase {
			// Default test case
			return &testcase{
				name:     "default t3.micro at 27% usage over 5m",
				interval: 5 * time.Minute,
				params:   params(),
				expRes:   2.2361292283536582,
			}
		}(),

		func() *testcase {
			// Calculate the default values over
			// a 30 second interval, instead of
			// 5 minutes
			return &testcase{
				name:     "default 30 second interval",
				interval: 30 * time.Second,
				params:   params(),
				expRes:   0.022361292283536588,
			}
		}(),

		func() *testcase {
			// Calculate the default values over
			// one hour
			return &testcase{
				name:     "1 hour interval",
				interval: 1 * time.Hour,
				params:   params(),
				expRes:   322.0026088829268,
			}
		}(),

		func() *testcase {
			// vCPUs not set in params, but set in metric
			p := params()
			p.vCPU = 0
			p.metric.SetUnitAmount(2).SetResourceUnit(v1.VCPU)
			return &testcase{
				name:     "vCPU set in metric, not params",
				interval: 5 * time.Minute,
				params:   p,
				expRes:   2.2361292283536582,
			}
		}(),
		func() *testcase {
			// vCPUs not set
			p := params()
			p.vCPU = 0
			return &testcase{
				name:     "vCPU not set",
				interval: 5 * time.Minute,
				params:   p,
				hasErr:   true,
				expErr:   "error vCPU set to 0",
				expRes:   0.00,
			}
		}(),
		func() *testcase {
			// calculate with 4 vCPUs
			p := params()
			p.vCPU = 4
			return &testcase{
				name:     "4 vCPU",
				interval: 5 * time.Minute,
				params:   p,
				expRes:   4.4722584567073165,
			}
		}(),

		func() *testcase {
			p := params()
			p.pue = 1.0
			// test if PUE is exactly 1
			return &testcase{
				name:     "PUE is exactly 1.0",
				interval: 5 * time.Minute,
				params:   p,
				expRes:   1.8634410236280485,
			}
		}(),

		func() *testcase {
			p := params()
			p.gridCO2e = 402
			// test an extremely high grid CO2e
			// This value was collected from azures
			// Germany West Central region
			return &testcase{
				name:     "High grid CO2e",
				interval: 5 * time.Minute,
				params:   p,
				expRes:   128.41770711402438,
			}
		}(),

		func() *testcase {
			// create a relatively large server with higher
			// than typical min and max watts, 32 vCPUs, and
			// a utilization of 90%
			p := params()
			p.metric.SetUnitAmount(32).SetUsage(90)
			return &testcase{
				name:     "large server and large workload",
				interval: 5 * time.Minute,
				params:   p,
				expRes:   3.9655170731707323,
			}
		}(),
	} {
		t.Run(test.name, func(t *testing.T) {
			res, err := cpu(test.interval, test.params)
			assert.Equalf(t, test.expRes, res, "Result should be: %v, got: %v", test.expRes, res)
			if test.hasErr {
				assert.Errorf(t, err, test.expErr)
			} else {
				assert.Nil(t, err)
			}
		})
	}
}

// Emission calculations should be over an interval of time,
// thus the same configurations over different intervals should
// have different values.
func TestIntervalTimes(t *testing.T) {
	t.Run("CPU emissions differ between 30s and 5 min", func(t *testing.T) {
		p := params()

		secs, err := cpu(30*time.Second, p)
		assert.Nil(t, err)

		mins, err := cpu(5*time.Minute, p)
		assert.Nil(t, err)

		assert.NotEqual(t, secs, mins)
	})

	t.Run("CPU emissions same between 60s and 1min", func(t *testing.T) {
		p := params()

		secs, err := cpu(60*time.Second, p)
		assert.Nil(t, err)

		mins, err := cpu(1*time.Minute, p)
		assert.Nil(t, err)

		assert.Equal(t, secs, mins)
	})
}

func TestCubicSplineInterpolation(t *testing.T) {
	type testcase struct {
		name    string
		wattage []data.Wattage
		usage   float64
		expRes  float64
		hasErr  bool
		expErr  string
	}
	testcases := []testcase{
		{
			name: "t3.micro instance at 27%",
			wattage: []data.Wattage{
				{
					Percentage: 0,
					Wattage:    1.21,
				},
				{
					Percentage: 10,
					Wattage:    3.05,
				},
				{
					Percentage: 50,
					Wattage:    7.16,
				},
				{
					Percentage: 100,
					Wattage:    9.96,
				},
			},
			usage:  27.00,
			expRes: 0.005324117210365854,
		},
		{
			name:    "empty wattage",
			wattage: []data.Wattage{},
			usage:   27.01,
			expRes:  0,
			hasErr:  true,
			expErr:  "error: cannot calculate CPU energy, no wattage found",
		},
		{
			name: "at exactly 10% utilization",
			wattage: []data.Wattage{
				{
					Percentage: 0,
					Wattage:    1.21,
				},
				{
					Percentage: 10,
					Wattage:    3.05,
				},
				{
					Percentage: 50,
					Wattage:    7.16,
				},
				{
					Percentage: 100,
					Wattage:    9.96,
				},
			},
			usage:  10,
			expRes: 0.0030499999999999993,
		},
	}

	for _, test := range testcases {
		t.Run(test.name, func(t *testing.T) {
			res, err := cubicSplineInterpolation(test.wattage, test.usage)
			if test.hasErr {
				assert.Errorf(t, err, test.expErr)
			} else {
				assert.Nil(t, err)
			}
			assert.Equalf(t, test.expRes, res, "Result should be: %v, got: %v", test.expRes, res)
		})
	}
}
