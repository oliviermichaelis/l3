package entry

import (
	"math"

	"github.com/oliviermichaelis/l3/pkg/ewma"

	"gonum.org/v1/gonum/stat/distuv"
)

type backend struct {
	latencyP50Seconds   ewma.ExponentialWeightedMovingAverage
	latencyP99Seconds   ewma.ExponentialWeightedMovingAverage
	latencyDistribution distuv.LogNormal
	requestsPerSecond   ewma.ExponentialWeightedMovingAverage
	successRate         ewma.ExponentialWeightedMovingAverage
	inFlightRequests    ewma.ExponentialWeightedMovingAverage
}

func (b *backend) calculateDistribution() {
	inverseP50 := normalCDFInverse(0.5)
	inverseP99 := normalCDFInverse(0.99)

	averageP50 := b.latencyP50Seconds.Value()
	averageP99 := b.latencyP99Seconds.Value()
	b.latencyDistribution.Sigma = (math.Log(averageP99) - math.Log(averageP50)) / (inverseP99 - inverseP50)
	b.latencyDistribution.Mu = (math.Log(averageP50)*inverseP99 - math.Log(averageP99)*inverseP50) / (inverseP99 - inverseP50)
}

func (b *backend) expectedValue() float64 {
	return b.latencyDistribution.Mean()
}

func rationalApproximation(t float64) float64 {
	// Abramowitz and Stegun formula 26.2.23.
	// The absolute value of the error should be less than 4.5 e-4.
	c := []float64{2.515517, 0.802853, 0.010328}
	d := []float64{1.432788, 0.189269, 0.001308}
	return t - ((c[2]*t+c[1])*t+c[0])/(((d[2]*t+d[1])*t+d[0])*t+1.0)
}

func normalCDFInverse(p float64) float64 {
	if p <= 0.0 || p >= 1.0 {
		panic("invalid argument")
	}

	if p < 0.5 {
		// F^-1(p) = - G^-1(p)
		return -rationalApproximation(math.Sqrt(-2.0 * math.Log(p)))
	}
	// F^-1(p) = G^-1(1-p)
	return rationalApproximation(math.Sqrt(-2.0 * math.Log(1-p)))
}
