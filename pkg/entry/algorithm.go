package entry

import (
	"math"
)

type Algorithm interface {
	backendWeight(b *backend) float64
}

// This algorithm is the original L3 algorithm
type InFlightLatency struct {
	failureLatencySeconds float64
}

func (i *InFlightLatency) backendWeight(b *backend) float64 {
	expectedSuccessLatency := b.expectedValue()
	successRate := b.successRate.Value()
	rps := b.requestsPerSecond.Value()

	// make sure we don't divide by zero
	var inflightRatio = 0.0
	if rps != 0.0 {
		inflightRatio = b.inFlightRequests.Value() / rps
	}

	var estimatedLatency float64
	if successRate == 1.0 {
		estimatedLatency = expectedSuccessLatency
	} else {
		estimatedLatency = expectedSuccessLatency + i.failureLatencySeconds*(1.0/successRate-1.0)
	}

	weight := 100.0 / (math.Pow(inflightRatio+1, 2) * estimatedLatency)
	if weight < 1.0 {
		weight = 1.0
	}

	return weight
}

func NewInFlightLatency(failureLatencySeconds float64) Algorithm {
	return &InFlightLatency{
		failureLatencySeconds: failureLatencySeconds,
	}
}

type C3 struct{}

func (c *C3) backendWeight(b *backend) float64 {
	rps := b.requestsPerSecond.Value()
	// make sure we don't divide by zero
	var inflightRatio = 0.0
	if rps != 0.0 {
		inflightRatio = b.inFlightRequests.Value() / rps
	}

	expectedLatency := b.expectedValue()
	// The rate is per second
	serviceRate := 1.0 / expectedLatency

	score := b.expectedValue() + math.Pow(inflightRatio, 3)/serviceRate

	// Return the inverse, scaled up to 1000
	return 1000.0 / score
}

// This algorithm is an improvement on the original L3 algorithm
type L3 struct {
	failureLatencySeconds float64
}

func (l *L3) backendWeight(b *backend) float64 {
	// b.latencyP99Seconds.Value()
	// expectedSuccessLatency := b.expectedValue()
	latencyP99 := b.latencyP99Seconds.Value()
	successRate := b.successRate.Value()
	rps := b.requestsPerSecond.Value()

	// make sure we don't divide by zero
	var inflightRatio = 0.0
	if rps != 0.0 {
		inflightRatio = b.inFlightRequests.Value() / rps
	}

	var estimatedLatency float64
	if successRate == 1.0 {
		estimatedLatency = latencyP99
	} else {
		estimatedLatency = latencyP99 + l.failureLatencySeconds*(1.0/successRate-1.0)
	}

	weight := 100.0 / (math.Pow(inflightRatio+1, 2) * estimatedLatency)
	if weight < 1.0 {
		weight = 1.0
	}

	return weight
}

func NewL3(failureLatencySeconds float64) Algorithm {
	return &L3{
		failureLatencySeconds: failureLatencySeconds,
	}
}
