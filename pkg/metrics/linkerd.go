package metrics

import (
	"context"
	"fmt"

	"github.com/linkerd/linkerd2/pkg/k8s"

	pb "github.com/linkerd/linkerd2/viz/metrics-api/gen/viz"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
)

type Linkerd interface {
	Retrieve(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) ([]*pb.StatTable_PodGroup_Row, error)
}

type linkerdMetrics struct {
	client     pb.ApiClient
	timeWindow string
}

func (l *linkerdMetrics) Retrieve(ctx context.Context, ts *trafficsplitv1alpha2.TrafficSplit) ([]*pb.StatTable_PodGroup_Row, error) {
	// Query used in the background:
	// histogram_quantile(0.99, sum(irate(response_latency_ms_bucket{authority=~\\\"^podinfo.podinfo.svc.*\\\", direction=\\\"outbound\\\"}[5s])) by (le, dst_namespace, dst_service))
	resp, err := l.client.StatSummary(ctx, &pb.StatSummaryRequest{
		Selector: &pb.ResourceSelection{
			Resource: &pb.Resource{
				Type:      k8s.TrafficSplit,
				Namespace: ts.Namespace,
				Name:      ts.Name,
			},
		},
		TimeWindow: l.timeWindow, // At all times we need to make sure the time window selects a sample
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get StatSummary: %w", err)
	}

	return podRows(resp)
}

func NewLinkerdMetrics(metricsApi pb.ApiClient, timeWindow string) *linkerdMetrics {
	return &linkerdMetrics{
		client:     metricsApi,
		timeWindow: timeWindow,
	}
}

func podRows(resp *pb.StatSummaryResponse) ([]*pb.StatTable_PodGroup_Row, error) {
	statTables := resp.GetOk().GetStatTables()
	if len(statTables) > 1 {
		return nil, fmt.Errorf("table has more than one Entry: %d", len(statTables))
	}

	pg, ok := statTables[0].Table.(*pb.StatTable_PodGroup_)
	if !ok {
		return nil, fmt.Errorf("could not type assert *StatTable_PodGroup_")
	}
	return pg.PodGroup.GetRows(), nil
}
