package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/oliviermichaelis/l3/pkg/ewma"

	prometheus "github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type InflightResponse struct {
	Service                string
	InflightRequestsSample ewma.Sample
}

type Prometheus interface {
	Retrieve(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) ([]InflightResponse, error)
}

type PrometheusMetrics struct {
	api prometheusv1.API
}

func (p *PrometheusMetrics) Retrieve(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) ([]InflightResponse, error) {
	logger := log.FromContext(ctx)

	if len(ts.Spec.Backends) == 0 {
		return nil, fmt.Errorf("trafficplits backend list is empty")
	}

	timestamp := time.Now()
	var responses []InflightResponse
	for _, b := range ts.Spec.Backends {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()

		labels := fmt.Sprintf("direction=\"outbound\",dst_namespace=\"%s\",dst_service=\"%s\"", ts.GetNamespace(), b.Service)
		query := fmt.Sprintf("sum(request_total{%s}) - sum(response_total{%s})", labels, labels)
		result, warnings, err := p.api.Query(ctx, query, timestamp)
		if err != nil {
			return nil, fmt.Errorf("unable to query inflight requests: %v", err)
		}
		for _, w := range warnings {
			logger.Info(w, "namespace", ts.GetNamespace(), "name", ts.GetName(), "backend", b.Service)
		}

		vector, ok := result.(model.Vector)
		if !ok {
			return nil, fmt.Errorf("result is not of type *model.Scalar but of type %T", result)
		}

		if vector.Len() > 1 {
			return []InflightResponse{}, fmt.Errorf("received unexpected vector length for backend %s: %d", b.Service, vector.Len())
		}

		var value float64
		if vector.Len() == 0 {
			value = 0.0
		} else {
			value = float64(vector[0].Value)
		}

		responses = append(responses, InflightResponse{
			Service: b.Service,
			InflightRequestsSample: ewma.Sample{
				Timestamp: timestamp,
				Value:     value,
			},
		})
	}

	return responses, nil
}

func NewPrometheusMetrics(address string) (*PrometheusMetrics, error) {
	client, err := prometheus.NewClient(prometheus.Config{
		Address: address,
		RoundTripper: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second, // Default value from the prometheus package
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second,
		},
	})
	if err != nil {
		return nil, err
	}

	return &PrometheusMetrics{
		api: prometheusv1.NewAPI(client),
	}, nil
}

type MockedPrometheus struct {
	response      []InflightResponse
	errorResponse error
}

func (m *MockedPrometheus) Retrieve(_ context.Context, _ *trafficsplitv1alpha2.TrafficSplit) ([]InflightResponse, error) {
	return m.response, m.errorResponse
}
