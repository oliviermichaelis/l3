package ewma

import (
	"testing"
	"time"
)

func TestEWMA_AddSample(t *testing.T) {
	t.Parallel()

	initialTime := time.Now()
	testcases := []struct {
		description   string
		ewma          *EWMA
		sample        Sample
		expectedValue float64
	}{
		{
			description: "newly initialized object without samples",
			ewma: &EWMA{
				value:              5.0,
				lastUpdate:         initialTime,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 0.1,
			},
			sample:        Sample{Value: 0.125, Timestamp: initialTime.Add(1 * time.Second)},
			expectedValue: 4.116312421255162,
		},
		{
			description: "increasing ewma",
			ewma: &EWMA{
				lastUpdate: initialTime,
				value:      0.100,
				decay:      5 * time.Second,
			},
			sample:        Sample{Value: 0.125, Timestamp: initialTime.Add(1 * time.Second)},
			expectedValue: 0.10453173117305047,
		},
		{
			description: "newly initialized object without samples",
			ewma: &EWMA{
				lastUpdate: initialTime,
				value:      0.100,
				decay:      5 * time.Second,
			},
			sample:        Sample{Value: 0.05, Timestamp: initialTime.Add(1 * time.Second)},
			expectedValue: 0.0909365376538991,
		},
	}

	for _, tc := range testcases {
		tc := tc // otherwise the underlying Value will change
		t.Run(tc.description, func(t *testing.T) {
			tc.ewma.AddSample(tc.sample)
			if tc.ewma.lastUpdate != tc.sample.Timestamp {
				t.Errorf("timestamp should have been: %v, but was: %v", tc.sample, tc.ewma.lastUpdate)
			}
			if tc.ewma.value != tc.expectedValue {
				t.Errorf("Value should have been: %v, but was: %v", tc.expectedValue, tc.ewma.value)
			}
		})
	}
}

func TestEWMA_MissingSample(t *testing.T) {
	initialTimestamp := time.Time{}
	tests := []struct {
		name          string
		ewma          *EWMA
		timestamp     time.Time
		expectedValue float64
	}{
		{
			name: "EWMA is above initial value",
			ewma: &EWMA{
				value:              10.0,
				lastUpdate:         initialTimestamp,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 1.0,
			},
			timestamp:     initialTimestamp.Add(5 * time.Second),
			expectedValue: 9.0,
		},
		{
			name: "EWMA is below initial value",
			ewma: &EWMA{
				value:              3.0,
				lastUpdate:         initialTimestamp,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 1.0,
			},
			timestamp:     initialTimestamp.Add(5 * time.Second),
			expectedValue: 4.0,
		},
		{
			name: "EWMA is below initial value and will be reset to the initial value",
			ewma: &EWMA{
				value:              4.5,
				lastUpdate:         initialTimestamp,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 1.0,
			},
			timestamp:     initialTimestamp.Add(5 * time.Second),
			expectedValue: 5.0,
		},
		{
			name: "EWMA is above initial value and will be reset to the initial value",
			ewma: &EWMA{
				value:              5.5,
				lastUpdate:         initialTimestamp,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 1.0,
			},
			timestamp:     initialTimestamp.Add(5 * time.Second),
			expectedValue: 5.0,
		},
		{
			name: "EWMA is equal to the initial value",
			ewma: &EWMA{
				value:              5.0,
				lastUpdate:         initialTimestamp,
				decay:              5 * time.Second,
				initial:            5.0,
				missingSampleDelta: 1.0,
			},
			timestamp:     initialTimestamp.Add(5 * time.Second),
			expectedValue: 5.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.ewma.MissingSample(tt.timestamp)

			if !tt.ewma.lastUpdate.Equal(tt.timestamp) {
				t.Errorf("last update is %v, but should have been %v", tt.ewma.lastUpdate, tt.timestamp)
			}

			if tt.ewma.value != tt.expectedValue {
				t.Errorf("value is %v, but should have been %v", tt.ewma.value, tt.expectedValue)
			}
		})
	}
}
