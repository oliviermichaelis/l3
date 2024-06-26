/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"path/filepath"
	"testing"
	"time"
	
	"github.com/oliviermichaelis/l3/pkg/entry"
	"github.com/oliviermichaelis/l3/pkg/ewma"
	"github.com/oliviermichaelis/l3/pkg/manager"
	"github.com/oliviermichaelis/l3/pkg/metrics"

	pb "github.com/linkerd/linkerd2/viz/metrics-api/gen/viz"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestAPIs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test suite")
	}
	RegisterFailHandler(Fail)

	RunSpecsWithDefaultAndCustomReporters(t,
		"Controller Suite",
		[]Reporter{printer.NewlineReporter{}})
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases"), filepath.Join("..", "test", "trafficsplit-crd.yaml")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = trafficsplitv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).ToNot(HaveOccurred())

	m := mockedMetricsClient{
		mockedStatSummaryResponse: &pb.StatSummaryResponse{
			Response: &pb.StatSummaryResponse_Ok_{
				Ok: &pb.StatSummaryResponse_Ok{
					StatTables: []*pb.StatTable{
						{
							Table: &pb.StatTable_PodGroup_{
								PodGroup: &pb.StatTable_PodGroup{
									Rows: []*pb.StatTable_PodGroup_Row{
										{
											Stats: &pb.BasicStats{
												LatencyMsP50: 50,
												LatencyMsP95: 100,
												LatencyMsP99: 200,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	reconciler := &TrafficSplitReconciler{
		Client:                 k8sManager.GetClient(),
		Scheme:                 k8sManager.GetScheme(),
		LabelControlledByValue: "least-latency",
	}
	linkerd := metrics.NewLinkerdMetrics(m, "10s")
	prometheus := &metrics.MockedPrometheus{}
	reconciler.Manager = manager.NewManager(context.TODO(), linkerd, prometheus, reconciler, entry.NewInFlightLatency(0.8), ewma.EWMAType)

	err = (reconciler).SetupWithManager(k8sManager)
	Expect(err).ToNot(HaveOccurred())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctrl.SetupSignalHandler())
		Expect(err).ToNot(HaveOccurred(), "failed to run manager")
	}()

}, 60)

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()

	// The following code is an ugly workaround for https://github.com/kubernetes-sigs/controller-runtime/issues/1571
	if err != nil {
		time.Sleep(4 * time.Second)
	}
	err = testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})

type mockedMetricsClient struct {
	mockedStatSummaryResponse *pb.StatSummaryResponse
	returnedError             error
	pb.ApiClient
}

func (m mockedMetricsClient) StatSummary(ctx context.Context, in *pb.StatSummaryRequest, opts ...grpc.CallOption) (*pb.StatSummaryResponse, error) {
	return m.mockedStatSummaryResponse, m.returnedError
}
