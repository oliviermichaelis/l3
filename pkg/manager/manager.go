package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oliviermichaelis/l3/pkg/entry"
	"github.com/oliviermichaelis/l3/pkg/ewma"
	"github.com/oliviermichaelis/l3/pkg/metrics"

	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	refreshChannelSize = 128
)

// The TrafficSplitUpdater interface is used to maintain a clean architecture by dependency inversion
type TrafficSplitUpdater interface {
	UpdateTrafficSplit(ctx context.Context, objectKey client.ObjectKey, weights map[string]float64) error
}

// The Manager protects the entries map, which requires synchronization
type Manager struct {
	sync.RWMutex      // The mutex is protecting the entries map
	entries           map[string]*entry.Entry
	refreshChannel    <-chan *entry.Entry // only consume from the channel, as we only want one function and goroutine that writes into the channel
	linkerdMetrics    metrics.Linkerd
	prometheusMetrics metrics.Prometheus
	updater           TrafficSplitUpdater
	updatePeriod      time.Duration
	algorithm         entry.Algorithm
	typeEWMA          ewma.TypeEWMA
}

func (m *Manager) Add(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) error {
	m.Lock()
	defer m.Unlock()

	logger := log.FromContext(ctx)
	objectKey := client.ObjectKeyFromObject(ts)
	if len(ts.Spec.Backends) == 0 {
		return fmt.Errorf("trafficsplit %s cannot be added as it doesn't have backends", objectKey.String())
	}

	if m.isManaged(ts) {
		return fmt.Errorf("trafficsplit %s cannot be added as it's already managed", objectKey.String())
	}

	m.entries[objectKey.String()] = entry.NewEntry(ts, m.algorithm, m.typeEWMA)
	logger.Info("started to manage trafficsplit", "namespace", ts.GetNamespace(), "name", ts.GetName())

	return nil
}

func (m *Manager) Remove(ctx context.Context, objectKey client.ObjectKey) error {
	m.Lock()
	defer m.Unlock()

	logger := log.FromContext(ctx)

	_, exists := m.entries[objectKey.String()]
	if !exists {
		return fmt.Errorf("failed to remove entry %s as object is not managed", objectKey.String())
	}

	delete(m.entries, objectKey.String())
	logger.Info("removed managed entry", "namespace", objectKey.Namespace, "name", objectKey.Name)

	return nil
}

func (m *Manager) SyncBackends(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) error {
	m.RLock()
	defer m.RUnlock()

	logger := log.FromContext(ctx)

	e := m.entries[client.ObjectKeyFromObject(ts).String()]
	backends := make(map[string]struct{})
	for _, b := range ts.Spec.Backends {
		backends[b.Service] = struct{}{}
	}

	// Add leafs that are in the object but not yet in the entry
	for b := range backends {
		if !e.HasBackend(b) {
			if err := e.AddBackend(b); err != nil {
				return err
			}
			logger.Info(fmt.Sprintf("added backend: %s", b), "namespace", ts.GetNamespace(), "name", ts.GetName())
		}
	}

	// Remove leafs that are not anymore in the object but still in the entry
	for _, leaf := range e.BackendNames() {
		if _, ok := backends[leaf]; !ok {
			e.RemoveBackend(leaf)
			logger.Info(fmt.Sprintf("removed backend: %s", leaf), "namespace", ts.GetNamespace(), "name", ts.GetName())
		}
	}

	return nil
}

func (m *Manager) IsManaged(ts *trafficsplitv1alpha2.TrafficSplit) bool {
	m.RLock()
	defer m.RUnlock()

	return m.isManaged(ts)
}

func (m *Manager) isManaged(ts *trafficsplitv1alpha2.TrafficSplit) bool {
	_, exists := m.entries[client.ObjectKeyFromObject(ts).String()]
	return exists
}

// enqueuePeriodically periodically enqueues all Entry in the entries map. It's supposed to be called in an own goroutine
// and stops once the passed context is cancelled
func (m *Manager) enqueuePeriodically(ctx context.Context, refresh chan<- *entry.Entry) {
	logger := log.FromContext(ctx)
	ticker := time.NewTicker(m.updatePeriod)

	for {
		select {
		case <-ctx.Done():
			logger.Info("enqueue was canceled", "reason", ctx.Err().Error())
			return
		case <-ticker.C:
			m.RLock()
			for _, e := range m.entries {
				refresh <- e
			}
			m.RUnlock()
		}
	}
}

func (m *Manager) dequeue(ctx context.Context) {
	logger := log.FromContext(ctx)
	errCh := make(chan dequeueError)

	for {
		select {
		case <-ctx.Done():
			logger.Info("dequeue was canceled", "reason", ctx.Err().Error())
			return
		case r := <-errCh:
			logger.Error(r.err, "unexpected failure when refreshing trafficsplit", "id", r.id.String())
		case e := <-m.refreshChannel:
			go m.refresh(ctx, errCh, e)
		}
	}
}

func (m *Manager) refresh(ctx context.Context, errCh chan dequeueError, e *entry.Entry) {
	requestCtx, requestCancelCtx := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancelCtx()

	// Retrieve the latest TrafficSplit stats
	rows, err := m.linkerdMetrics.Retrieve(requestCtx, e.TrafficSplit())
	if err != nil {
		errCh <- dequeueError{
			err: err,
			id:  e.ObjectKey(),
		}
		return
	}

	latencyChanged, err := e.UpdateLatency(rows)
	if err != nil {
		errCh <- dequeueError{
			err: err,
			id:  e.ObjectKey(),
		}
		return
	}

	// Retrieve in-flight requests from prometheus
	responses, err := m.prometheusMetrics.Retrieve(ctx, e.TrafficSplit())
	if err != nil {
		errCh <- dequeueError{
			err: err,
			id:  e.ObjectKey(),
		}
		return
	}

	// Add retrieved in-flight samples to the EWMA
	var inflightChanged bool
	for _, r := range responses {
		changed, err := e.UpdateInflightRequests(r.Service, r.InflightRequestsSample)
		if err != nil {
			errCh <- dequeueError{
				err: err,
				id:  e.ObjectKey(),
			}
			return
		}

		if changed {
			inflightChanged = true
		}
	}

	// Skip updating the TrafficSplit, as there are no changes
	if !latencyChanged && !inflightChanged {
		return
	}

	weights := e.CalculateWeights()

	if err := m.updater.UpdateTrafficSplit(ctx, e.ObjectKey(), weights); err != nil {
		errCh <- dequeueError{
			err: err,
			id:  e.ObjectKey(),
		}
		return
	}
}

func NewManager(ctx context.Context, linkerdMetrics metrics.Linkerd, prometheusMetrics metrics.Prometheus, updater TrafficSplitUpdater, weightingAlgorithm entry.Algorithm, typeEWMA ewma.TypeEWMA) *Manager {
	refreshChannel := make(chan *entry.Entry, refreshChannelSize)
	m := &Manager{
		entries:           make(map[string]*entry.Entry),
		refreshChannel:    refreshChannel,
		linkerdMetrics:    linkerdMetrics,
		prometheusMetrics: prometheusMetrics,
		updater:           updater,
		updatePeriod:      5 * time.Second,
		algorithm:         weightingAlgorithm,
		typeEWMA:          typeEWMA,
	}

	go m.enqueuePeriodically(ctx, refreshChannel)
	go m.dequeue(ctx)

	return m
}

type dequeueError struct {
	err error
	id  client.ObjectKey
}
