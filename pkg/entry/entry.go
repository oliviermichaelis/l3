package entry

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/oliviermichaelis/l3/pkg/ewma"

	pb "github.com/linkerd/linkerd2/viz/metrics-api/gen/viz"
	"github.com/prometheus/client_golang/prometheus"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"gonum.org/v1/gonum/stat/distuv"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	initialLatency           = 5.0
	initialInflightRequests  = 100.0
	initialSuccessRate       = 1.0
	initialRequestsPerSecond = 0.0

	decayLatency           = 5 * time.Second
	decayInflightRequests  = 5 * time.Second
	decaySuccessRate       = 10 * time.Second
	decayRequestsPerSecond = 10 * time.Second

	deltaLatency           = 0.1 // 100ms
	deltaInflightRequests  = 1.0
	deltaSuccessRate       = 0.01
	deltaRequestsPerSecond = 10.0

	MetricsLabelNamespace = "namespace"
	MetricLabelName       = "name"
	MetricLabelBackend    = "backend"
)

var (
	RequestRateControlEnabled = true
)

// Prometheus metrics
var (
	expectedValueMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "expected_value_seconds",
		Help:      "The expected value of the log normal distribution of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	ewmaP50Metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_p50_seconds",
		Help:      "The peak EWMA of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	ewmaP99Metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_p99_seconds",
		Help:      "The peak EWMA of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	ewmaRequestsPerSecondEntry = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_requests_per_second_trafficsplit",
		Help:      "The RPS EWMA of the TrafficSplit",
	}, []string{MetricsLabelNamespace, MetricLabelName})
	ewmaRequestsPerSecondBackend = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_requests_per_second",
		Help:      "The RPS EWMA of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	ewmaSuccessRateMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_success_rate",
		Help:      "The success rate EWMA of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	ewmaInflightRequestsMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "ewma_inflight_requests",
		Help:      "The inflight requests EWMA of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	latencyP50Metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "latency_p50_seconds",
		Help:      "The 50th percentile latency of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	latencyP95Metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "latency_p95_seconds",
		Help:      "The 95th percentile latency of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	latencyP99Metric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "latency_p99_seconds",
		Help:      "The 99th percentile latency of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	requestsPerSecondEntry = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "requests_per_second_trafficsplit",
		Help:      "The RPS to the trafficsplit",
	}, []string{MetricsLabelNamespace, MetricLabelName})
	requestsPerSecondBackend = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "requests_per_second",
		Help:      "The RPS of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	successRateMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "success_rate",
		Help:      "The success rate of the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
	inflightRequestsMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "inflight_requests",
		Help:      "The number of inflight requests to the backend",
	}, []string{MetricsLabelNamespace, MetricLabelName, MetricLabelBackend})
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		expectedValueMetric,
		ewmaP50Metric,
		ewmaP99Metric,
		ewmaRequestsPerSecondEntry,
		ewmaRequestsPerSecondBackend,
		ewmaSuccessRateMetric,
		ewmaInflightRequestsMetric,
		latencyP50Metric,
		latencyP95Metric,
		latencyP99Metric,
		requestsPerSecondEntry,
		requestsPerSecondBackend,
		successRateMetric,
		inflightRequestsMetric,
	)
}

type Entry struct {
	mu                    sync.RWMutex // the mutex protects the entire Entry struct, including the backends
	trafficSplit          *trafficsplitv1alpha2.TrafficSplit
	backends              map[string]*backend
	ewmaRequestsPerSecond ewma.ExponentialWeightedMovingAverage
	lastRequestsPerSecond float64
	typeEWMA              ewma.TypeEWMA
	Algorithm
}

func (e *Entry) TrafficSplit() *trafficsplitv1alpha2.TrafficSplit {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.trafficSplit
}

func (e *Entry) UpdateLatency(podRows []*pb.StatTable_PodGroup_Row) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	sampleTime := time.Now()
	totalRequestsPerSecond := 0.0
	changed := false
	for _, leafRow := range podRows {
		stats := leafRow.GetStats()
		leaf := leafRow.GetTsStats().GetLeaf()

		b, ok := e.backends[leaf]
		if !ok {
			return false, fmt.Errorf("leaf service in Entry not found: %s", leaf)
		}

		timeWindow, err := time.ParseDuration(leafRow.TimeWindow)
		if err != nil {
			return false, fmt.Errorf("unable to parse time window: %v", err)
		}

		oldP50, oldP99 := b.latencyP50Seconds.Value(), b.latencyP99Seconds.Value()
		if stats == nil || (stats.LatencyMsP99 == 0 && stats.SuccessCount == 0 && stats.FailureCount == 0) {
			// The API is returning nil stats or zero latency, as there is likely no traffic in the time window
			b.latencyP50Seconds.MissingSample(sampleTime)
			b.latencyP99Seconds.MissingSample(sampleTime)
			b.requestsPerSecond.AddSample(ewma.Sample{Value: 0.0, Timestamp: sampleTime})
			b.successRate.MissingSample(sampleTime)
		} else {
			rps := float64(stats.SuccessCount+stats.FailureCount) / timeWindow.Seconds()
			totalRequestsPerSecond += rps

			// Recalculate EWMA with the latest sample
			b.latencyP50Seconds.AddSample(ewma.Sample{Value: float64(stats.LatencyMsP50) / 1000.0, Timestamp: sampleTime})
			b.latencyP99Seconds.AddSample(ewma.Sample{Value: float64(stats.LatencyMsP99) / 1000.0, Timestamp: sampleTime})
			b.requestsPerSecond.AddSample(ewma.Sample{Value: rps, Timestamp: sampleTime})

			successRate, err := successRateFromStats(stats)
			if err != nil {
				return false, err
			}
			b.successRate.AddSample(ewma.Sample{Value: successRate, Timestamp: sampleTime})
		}

		// Update latency metrics with the latest metrics fetched from metrics-api
		linkerdMetrics(stats, e.objectKey(), leaf, timeWindow)

		// Determine and set the log-normal distribution parameters
		b.calculateDistribution()

		ewmaP50Metric.With(prometheus.Labels{
			MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
			MetricLabelName:       e.trafficSplit.GetName(),
			MetricLabelBackend:    leaf,
		}).Set(b.latencyP50Seconds.Value())
		ewmaP99Metric.With(prometheus.Labels{
			MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
			MetricLabelName:       e.trafficSplit.GetName(),
			MetricLabelBackend:    leaf,
		}).Set(b.latencyP99Seconds.Value())
		ewmaRequestsPerSecondBackend.With(prometheus.Labels{
			MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
			MetricLabelName:       e.trafficSplit.GetName(),
			MetricLabelBackend:    leaf,
		}).Set(b.requestsPerSecond.Value())
		ewmaSuccessRateMetric.With(prometheus.Labels{
			MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
			MetricLabelName:       e.trafficSplit.GetName(),
			MetricLabelBackend:    leaf,
		}).Set(b.successRate.Value())
		expectedValueMetric.With(prometheus.Labels{
			MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
			MetricLabelName:       e.trafficSplit.GetName(),
			MetricLabelBackend:    leaf,
		}).Set(b.expectedValue())

		if oldP50 != b.latencyP50Seconds.Value() || oldP99 != b.latencyP99Seconds.Value() {
			changed = true
		}
	}

	e.ewmaRequestsPerSecond.AddSample(ewma.Sample{Value: totalRequestsPerSecond, Timestamp: sampleTime})
	e.lastRequestsPerSecond = totalRequestsPerSecond
	ewmaRequestsPerSecondEntry.With(prometheus.Labels{
		MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
		MetricLabelName:       e.trafficSplit.GetName(),
	}).Set(e.ewmaRequestsPerSecond.Value())
	requestsPerSecondEntry.With(prometheus.Labels{
		MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
		MetricLabelName:       e.trafficSplit.GetName(),
	}).Set(totalRequestsPerSecond)

	return changed, nil
}

func (e *Entry) UpdateInflightRequests(backend string, sample ewma.Sample) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, ok := e.backends[backend]
	if !ok {
		return false, fmt.Errorf("backend entry %s does not exist", backend)
	}

	changed := false
	if b.inFlightRequests.Value() != sample.Value {
		changed = true
	}

	e.backends[backend].inFlightRequests.AddSample(sample)

	ewmaInflightRequestsMetric.With(prometheus.Labels{
		MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
		MetricLabelName:       e.trafficSplit.GetName(),
		MetricLabelBackend:    backend,
	}).Set(b.inFlightRequests.Value())
	inflightRequestsMetric.With(prometheus.Labels{
		MetricsLabelNamespace: e.trafficSplit.GetNamespace(),
		MetricLabelName:       e.trafficSplit.GetName(),
		MetricLabelBackend:    backend,
	}).Set(sample.Value)

	return changed, nil
}

// ObjectKey returns the key used to unambiguously identify the Entry and its TrafficSplit.
// This method should only be called from outside the package due to the mutex
func (e *Entry) ObjectKey() client.ObjectKey {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.objectKey()
}

// objectKey is only for package internal use
func (e *Entry) objectKey() client.ObjectKey {
	return client.ObjectKeyFromObject(e.trafficSplit)
}

func (e *Entry) BackendNames() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	backends := make([]string, len(e.backends))
	i := 0
	for k := range e.backends {
		backends[i] = k
		i++
	}

	return backends
}

func (e *Entry) HasBackend(id string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.hasBackend(id)
}

func (e *Entry) hasBackend(id string) bool {
	_, ok := e.backends[id]
	return ok
}

func (e *Entry) AddBackend(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.hasBackend(id) {
		return fmt.Errorf("backend already exists with name %s", id)
	}

	e.backends[id] = &backend{
		latencyP50Seconds:   ewma.NewEWMA(initialLatency, decayLatency, deltaLatency),
		latencyP99Seconds:   ewma.NewEWMA(initialLatency, decayLatency, deltaLatency),
		latencyDistribution: distuv.LogNormal{},
		requestsPerSecond:   ewma.NewEWMA(initialRequestsPerSecond, decayRequestsPerSecond, deltaRequestsPerSecond),
		successRate:         ewma.NewEWMA(initialSuccessRate, decaySuccessRate, deltaSuccessRate),
		inFlightRequests:    ewma.NewEWMA(initialInflightRequests, decayInflightRequests, deltaInflightRequests),
	}

	return nil
}

func (e *Entry) RemoveBackend(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.backends, id)
}

func (e *Entry) CalculateWeights() map[string]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	response := make(map[string]float64)
	for name, b := range e.backends {
		weight := e.backendWeight(b)
		response[name] = weight
	}

	if RequestRateControlEnabled {
		response = e.requestsRateControl(response)
	}

	return response
}

// requestsRateControl adjusts the weights according to RPS changes
// * Ratios of the weights are increased if the RPS has decreased in order to opportunistically shift traffic to the fastest backend
// * Ratios of the weights are equalized if the RPS has increased in order to share load more equally between backends and allow for autoscaling to kick in
func (e *Entry) requestsRateControl(weights map[string]float64) map[string]float64 {
	if len(weights) <= 1 { // With less than two weights, there is no point in changing it
		return weights
	}

	rps := e.ewmaRequestsPerSecond.Value()
	// The relativeChange is used to determine whether to increase/decrease traffic to slow backends
	relativeChange := (e.lastRequestsPerSecond - rps) / rps
	if e.lastRequestsPerSecond == 0.0 { // In case it's zero, we don't want to change weights
		relativeChange = 0.0
	}

	// Calculate the average of all weights
	var average float64
	for _, w := range weights {
		average += w
	}
	average = average / float64(len(weights))

	// The weights are adjusted according to the RPS change
	for i, w := range weights {
		// A composite function is used here to make sure that our subtraction never results in a negative result
		// https://www.desmos.com/calculator/ub74g380fa
		var weight float64
		if relativeChange > 0 {
			weight = average - average/math.Pow(1+math.Pow(relativeChange, 2), 3/2) + w/math.Pow(1+math.Pow(relativeChange, 2), 3/2)
		} else {
			if w <= average {
				weight = w / math.Pow(1+math.Pow(2*relativeChange, 2), 3/2)
			} else {
				weight = 2*w - average - (w-average)/math.Pow(1+math.Pow(3*relativeChange, 2), 3/2)
			}
		}

		if weight < 1.0 {
			weight = 1.0
		}
		weights[i] = weight
	}

	return weights
}

func NewEntry(ts *trafficsplitv1alpha2.TrafficSplit, weightingAlgorithm Algorithm, typeEWMA ewma.TypeEWMA) *Entry {
	e := &Entry{
		trafficSplit: ts,
		backends:     make(map[string]*backend),
		typeEWMA:     typeEWMA,
		Algorithm:    weightingAlgorithm,
	}

	if typeEWMA == ewma.EWMAType {
		e.ewmaRequestsPerSecond = ewma.NewEWMA(initialRequestsPerSecond, decayRequestsPerSecond, deltaRequestsPerSecond)
	} else if typeEWMA == ewma.PeakEWMAType {
		e.ewmaRequestsPerSecond = ewma.NewPeakEWMA(initialRequestsPerSecond, decayRequestsPerSecond, deltaRequestsPerSecond)
	}

	return e
}

func linkerdMetrics(stats *pb.BasicStats, objectKey client.ObjectKey, backend string, timeWindow time.Duration) {
	// The *pb.BasicStats can be nil if the metrics-api cannot provide metrics
	if stats == nil {
		return
	}

	if stats.LatencyMsP50 != 0 {
		latencyP50Metric.With(prometheus.Labels{
			MetricsLabelNamespace: objectKey.Namespace,
			MetricLabelName:       objectKey.Name,
			MetricLabelBackend:    backend,
		}).Set(float64(stats.LatencyMsP50) / 1000)
	}
	if stats.LatencyMsP95 != 0 {
		latencyP95Metric.With(prometheus.Labels{
			MetricsLabelNamespace: objectKey.Namespace,
			MetricLabelName:       objectKey.Name,
			MetricLabelBackend:    backend,
		}).Set(float64(stats.LatencyMsP95) / 1000)
	}
	if stats.LatencyMsP99 != 0 {
		latencyP99Metric.With(prometheus.Labels{
			MetricsLabelNamespace: objectKey.Namespace,
			MetricLabelName:       objectKey.Name,
			MetricLabelBackend:    backend,
		}).Set(float64(stats.LatencyMsP99) / 1000)
	}

	requestsPerSecondBackend.With(prometheus.Labels{
		MetricsLabelNamespace: objectKey.Namespace,
		MetricLabelName:       objectKey.Name,
		MetricLabelBackend:    backend,
	}).Set(float64(stats.SuccessCount+stats.FailureCount) / timeWindow.Seconds())

	successRate, err := successRateFromStats(stats)
	if err == nil {
		successRateMetric.With(prometheus.Labels{
			MetricsLabelNamespace: objectKey.Namespace,
			MetricLabelName:       objectKey.Name,
			MetricLabelBackend:    backend,
		}).Set(successRate)
	}
}

func successRateFromStats(stats *pb.BasicStats) (float64, error) {
	total := float64(stats.SuccessCount + stats.FailureCount)
	if total == 0.0 {
		return 0, fmt.Errorf("total number of requests for success rate is 0")
	}
	return float64(stats.SuccessCount) / total, nil
}
