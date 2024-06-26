package ewma

import "time"

const (
	PeakEWMAType TypeEWMA = "peakewma"
)

type PeakEWMA struct {
	*EWMA
}

func (p *PeakEWMA) AddSample(sample Sample) {
	if sample.Value > p.value {
		p.value = sample.Value
		p.lastUpdate = sample.Timestamp
	}

	p.EWMA.AddSample(sample)
}

func (p *PeakEWMA) MissingSample(timestamp time.Time) {
	p.EWMA.MissingSample(timestamp)
}

func (p *PeakEWMA) Value() float64 {
	return p.EWMA.Value()
}

func NewPeakEWMA(initialValue float64, decay time.Duration, missingSampleDelta float64) ExponentialWeightedMovingAverage {
	return &PeakEWMA{
		&EWMA{
			value:              initialValue,
			lastUpdate:         time.Now(),
			decay:              decay,
			initial:            initialValue,
			missingSampleDelta: missingSampleDelta,
		},
	}
}
