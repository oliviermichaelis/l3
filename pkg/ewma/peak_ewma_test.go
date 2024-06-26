package ewma

import (
	"testing"
	"time"
)

func TestPeakEWMA_AddSample(t *testing.T) {
	sampleTime := time.Now()
	e := EWMA{
		value:              5.0,
		lastUpdate:         sampleTime.Add(-1 * time.Second),
		decay:              1 * time.Second,
		initial:            1.0,
		missingSampleDelta: 0.5,
	}

	tc := []struct {
		description   string
		peakEWMA      *PeakEWMA
		sample        Sample
		expectedValue float64
	}{
		{
			description: "sample value is lower than EWMA value",
			peakEWMA:    &PeakEWMA{&e},
			sample: Sample{
				Value:     3,
				Timestamp: sampleTime,
			},
			expectedValue: 3.7357588823428847,
		},
		{
			description: "sample value is higher than EWMA value",
			peakEWMA:    &PeakEWMA{&e},
			sample: Sample{
				Value:     6,
				Timestamp: sampleTime,
			},
			expectedValue: 6,
		},
	}

	for _, tc := range tc {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			//tc.peakEWMA.AddSample(Sample{Value: 1, Timestamp: tc.sample.Timestamp.Add(-1 * time.Second)})
			tc.peakEWMA.AddSample(tc.sample)
			if v := tc.peakEWMA.Value(); v != tc.expectedValue {
				t.Errorf("expected value %v, but result is %v", tc.expectedValue, v)
			}
			if ts := tc.peakEWMA.EWMA.lastUpdate; !ts.Equal(tc.sample.Timestamp) {
				t.Errorf("expected last update timestamp %v but was %v", tc.sample.Timestamp, ts)
			}
		})
	}
}
