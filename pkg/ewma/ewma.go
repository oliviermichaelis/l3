package ewma

import (
	"fmt"
	"math"
	"time"
)

type TypeEWMA string

const (
	EWMAType TypeEWMA = "ewma"
)

type ExponentialWeightedMovingAverage interface {
	AddSample(sample Sample)
	MissingSample(timestamp time.Time)
	Value() float64
}

// EWMA is an Exponentially Weighted Moving Average implementation
type EWMA struct {
	value      float64
	lastUpdate time.Time // in UTC time
	decay      time.Duration
	initial    float64

	// missingSampleDelta is the value by which the EWMA should converge towards the initial value.
	// It has to be greater than zero.
	missingSampleDelta float64
}

func (e *EWMA) AddSample(sample Sample) {
	oldValue := e.value
	secondsElapsedSinceLastSample := sample.Timestamp.Sub(e.lastUpdate).Seconds()
	decay := math.Exp(-secondsElapsedSinceLastSample / e.decay.Seconds())
	e.lastUpdate = sample.Timestamp
	e.value = sample.Value*(1.0-decay) + oldValue*decay
	if e.value < 0.0 {
		panic(fmt.Errorf("EWMA value is below zero with sample value: %v and ewma: %v", sample.Value, e.value))
	}
}

// MissingSample is used to indicate that metric data is missing.
// The EWMA will be increased until the initial value is reached
func (e *EWMA) MissingSample(timestamp time.Time) {
	e.lastUpdate = timestamp

	if e.value > e.initial { // We need to converge to the initial value from above
		e.value = e.value - e.missingSampleDelta
		if e.value < e.initial { // We have overshot the initial value
			e.value = e.initial
		}
	} else if e.value < e.initial { // We need to converge to the initial value from below
		e.value = e.value + e.missingSampleDelta
		if e.value > e.initial { // We have overshot the initial value
			e.value = e.initial
		}
	}
}

func (e *EWMA) Value() float64 {
	return e.value
}

func NewEWMA(initialValue float64, decay time.Duration, missingSampleDelta float64) ExponentialWeightedMovingAverage {
	if missingSampleDelta <= 0.0 {
		panic(fmt.Errorf("missing sample delta is <= 0 with value: %v", missingSampleDelta))
	}

	return &EWMA{
		value:              initialValue,
		lastUpdate:         time.Now(),
		decay:              decay,
		initial:            initialValue,
		missingSampleDelta: missingSampleDelta,
	}
}
