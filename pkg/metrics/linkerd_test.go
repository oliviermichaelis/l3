package metrics

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/linkerd/linkerd2/viz/metrics-api/gen/viz"
	"google.golang.org/grpc"
)

type mockedApiClient struct {
	responseToReturn *pb.StatSummaryResponse
	errorToReturn    error
}

func (m mockedApiClient) StatSummary(ctx context.Context, in *pb.StatSummaryRequest, opts ...grpc.CallOption) (*pb.StatSummaryResponse, error) {
	return m.responseToReturn, m.errorToReturn
}

func (m mockedApiClient) Edges(ctx context.Context, in *pb.EdgesRequest, opts ...grpc.CallOption) (*pb.EdgesResponse, error) {
	panic("implement me")
}

func (m mockedApiClient) Gateways(ctx context.Context, in *pb.GatewaysRequest, opts ...grpc.CallOption) (*pb.GatewaysResponse, error) {
	panic("implement me")
}

func (m mockedApiClient) TopRoutes(ctx context.Context, in *pb.TopRoutesRequest, opts ...grpc.CallOption) (*pb.TopRoutesResponse, error) {
	panic("implement me")
}

func (m mockedApiClient) ListPods(ctx context.Context, in *pb.ListPodsRequest, opts ...grpc.CallOption) (*pb.ListPodsResponse, error) {
	panic("implement me")
}

func (m mockedApiClient) ListServices(context.Context, *pb.ListServicesRequest, ...grpc.CallOption) (*pb.ListServicesResponse, error) {
	panic("implement me")
}

func (m mockedApiClient) SelfCheck(context.Context, *pb.SelfCheckRequest, ...grpc.CallOption) (*pb.SelfCheckResponse, error) {
	panic("implement me")
}

func TestLinkerdMetrics_Retrieve(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description string
		client      pb.ApiClient
		isError     bool
	}{
		{
			description: "return error",
			client: &mockedApiClient{
				errorToReturn: fmt.Errorf("this is a mocked error"),
			},
			isError: true,
		},
		{
			description: "return empty response",
			client: &mockedApiClient{
				responseToReturn: &pb.StatSummaryResponse{},
			},
			isError: false,
		},
	}

	for _, tc := range testcases {
		tc := tc // otherwise the underlying value will change
		t.Run(tc.description, func(t *testing.T) {
			resp, err := tc.client.StatSummary(context.Background(), &pb.StatSummaryRequest{})
			if tc.isError && err == nil {
				t.Errorf("should have returned an error")
			} else if !tc.isError && err != nil {
				t.Errorf("should not have returned an error: %v", err)
			} else if tc.isError {
				return
			}

			if resp == nil {
				t.Errorf("response is nil")
			}
		})
	}
}
