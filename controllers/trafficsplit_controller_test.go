package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	trafficsplit "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("TrafficSplit controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		TrafficSplitName        = "test-trafficsplit"
		TrafficSplitNamespace   = "default"
		labelControlledByValues = "least-latency"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	var (
		originalBackends = []trafficsplitv1alpha2.TrafficSplitBackend{
			{
				Service: "leaf-service-1",
				Weight:  1,
			},
			{
				Service: "leaf-service-2",
				Weight:  1,
			},
		}
	)

	Context("When updating TrafficSplit controlled-by label", func() {
		It("Should start updating the weights", func() {
			By("By creating a TrafficSplit resource ")
			ctx := context.Background()
			trafficSplit := &trafficsplitv1alpha2.TrafficSplit{
				TypeMeta: metav1.TypeMeta{
					APIVersion: fmt.Sprintf("%s/%s", trafficsplit.GroupName, "v1alpha2"),
					Kind:       "TrafficSplit",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      TrafficSplitName,
					Namespace: TrafficSplitNamespace,
				},
				Spec: trafficsplitv1alpha2.TrafficSplitSpec{
					Service:  "apex-service",
					Backends: originalBackends,
				},
			}
			Expect(k8sClient.Create(ctx, trafficSplit)).Should(Succeed())

			trafficSplitLookupKey := types.NamespacedName{Name: TrafficSplitName, Namespace: TrafficSplitNamespace}
			createdTrafficSplit := &trafficsplitv1alpha2.TrafficSplit{}

			// We'll need to retry getting this newly created TrafficSplit, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, trafficSplitLookupKey, createdTrafficSplit)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			_, ok := createdTrafficSplit.GetLabels()[LabelControlledByKey]
			Expect(ok).Should(Equal(false))

			By("Set controlled-by label")
			trafficSplit.Labels = map[string]string{
				LabelControlledByKey: labelControlledByValues,
			}
			Expect(k8sClient.Update(ctx, trafficSplit)).Should(Succeed())

			Eventually(func() bool {
				err := k8sClient.Get(ctx, trafficSplitLookupKey, createdTrafficSplit)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			Expect(createdTrafficSplit.GetLabels()[LabelControlledByKey]).Should(Equal(labelControlledByValues))

			By("Checking if the original weight annotation is set")
			backends, err := json.Marshal(originalBackends)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, trafficSplitLookupKey, createdTrafficSplit)).NotTo(HaveOccurred())
				if _, ok := createdTrafficSplit.GetAnnotations()[AnnotationOriginalWeights]; ok {
					return true
				}
				return false
			}, timeout, interval).Should(BeTrue())

			Expect(createdTrafficSplit.GetAnnotations()[AnnotationOriginalWeights]).Should(Equal(string(backends)))
		})
	})
})
