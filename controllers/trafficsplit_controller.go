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
	"encoding/json"
	"fmt"

	"github.com/oliviermichaelis/l3/pkg/entry"
	"github.com/oliviermichaelis/l3/pkg/manager"

	"github.com/prometheus/client_golang/prometheus"
	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	LabelControlledByKey      = "least-latency.oliviermichaelis.dev/controlled-by"
	AnnotationOriginalWeights = "least-latency.oliviermichaelis.dev/original-weights"
)

var (
	weightMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "oliviermichaelis",
		Subsystem: "linkerdleastlatency",
		Name:      "weight",
		Help:      "The weight of the backend of the TrafficSplit",
	}, []string{entry.MetricsLabelNamespace, entry.MetricLabelName, entry.MetricLabelBackend})
)

func init() {
	ctrlmetrics.Registry.MustRegister(weightMetric)
}

// TrafficSplitReconciler reconciles a TrafficSplit object
type TrafficSplitReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	Manager                *manager.Manager
	LabelControlledByValue string
}

//+kubebuilder:rbac:groups=split.smi-spec.io,resources=trafficsplits,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=split.smi-spec.io,resources=trafficsplits/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=split.smi-spec.io,resources=trafficsplits/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *TrafficSplitReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Retrieve TrafficSplit from the api server
	trafficSplit := &trafficsplitv1alpha2.TrafficSplit{}
	if err := r.Get(ctx, req.NamespacedName, trafficSplit); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.Manager.Remove(ctx, req.NamespacedName); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err // Requeue again
	}

	// Make sure the TrafficSplit is still controlled by this controller, otherwise remove it
	if v, ok := trafficSplit.GetLabels()[LabelControlledByKey]; ok && v != r.LabelControlledByValue {
		// TrafficSplit is controlled by another operator
		return ctrl.Result{}, r.Manager.Remove(ctx, req.NamespacedName)
	} else if !ok {
		// TrafficSplit is not controlled by any operator anymore
		if err := r.Manager.Remove(ctx, req.NamespacedName); err != nil {
			return ctrl.Result{}, err
		}
		// TODO revert the weights to the values of the annotation if possible. And remove annotation afterwards
	}

	// Check if we already observe the TrafficSplit
	if r.Manager.IsManaged(trafficSplit) {
		// Check if the backends have changed
		if err := r.Manager.SyncBackends(ctx, trafficSplit); err != nil {
			return ctrl.Result{}, err
		}

		// The TrafficSplit is observed, we return without requeuing
		return ctrl.Result{}, nil
	}

	if err := r.Manager.Add(ctx, trafficSplit); err != nil {
		return ctrl.Result{}, err
	}

	// Register the backends internally
	if err := r.Manager.SyncBackends(ctx, trafficSplit); err != nil {
		return ctrl.Result{}, err
	}

	b, err := json.Marshal(trafficSplit.Spec.Backends)
	if err != nil {
		return ctrl.Result{Requeue: true}, err
	}
	annotation := trafficSplit.GetAnnotations()
	if annotation == nil {
		annotation = make(map[string]string)
	}
	annotation[AnnotationOriginalWeights] = string(b)
	trafficSplit.SetAnnotations(annotation)
	if err := r.Update(ctx, trafficSplit); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *TrafficSplitReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&trafficsplitv1alpha2.TrafficSplit{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(event event.CreateEvent) bool {
				if event.Object.GetLabels()[LabelControlledByKey] == r.LabelControlledByValue {
					return true
				}
				return false
			},
			DeleteFunc: func(deleteEvent event.DeleteEvent) bool {
				return true
			},
			UpdateFunc: func(updateEvent event.UpdateEvent) bool {
				if updateEvent.ObjectOld.GetLabels()[LabelControlledByKey] == r.LabelControlledByValue {
					// The object might have opted out of the controller, thus we need to process it
					return true
				} else if updateEvent.ObjectNew.GetLabels()[LabelControlledByKey] == r.LabelControlledByValue {
					// The object opted into the controller, thus we need to process it
					return true
				}
				return false
			},
			GenericFunc: func(genericEvent event.GenericEvent) bool {
				if genericEvent.Object.GetLabels()[LabelControlledByKey] == r.LabelControlledByValue {
					return true
				}
				return false
			},
		}).
		Complete(r)
}

func (r *TrafficSplitReconciler) UpdateTrafficSplit(ctx context.Context, objectKey client.ObjectKey, weights map[string]float64) error {
	logger := log.FromContext(ctx)

	// Get TrafficSplit from apiServer
	ts := &trafficsplitv1alpha2.TrafficSplit{}
	if err := r.Get(ctx, objectKey, ts); err != nil {
		return fmt.Errorf("failed to get TrafficSplit: %w", err)
	}

	// Check that it's still managed by this controller
	if !r.isControlled(ts) {
		return fmt.Errorf("trafficsplit is not managed by this controller anymore")
	}

	if len(weights) != len(ts.Spec.Backends) {
		logger.Info(fmt.Sprintf("received %d weights for %d backends", len(weights), len(ts.Spec.Backends)), "namespace", ts.GetNamespace(), "name", ts.GetName())
	}

	for i, backend := range ts.Spec.Backends {
		w, ok := weights[backend.Service]
		if !ok {
			logger.Error(fmt.Errorf("failed to find weight for backend: %s", backend.Service), "namespace", ts.GetNamespace(), "name", ts.GetName())
			continue
		}
		ts.Spec.Backends[i].Weight = int(w)

		weightMetric.With(prometheus.Labels{
			entry.MetricsLabelNamespace: objectKey.Namespace,
			entry.MetricLabelName:       objectKey.Name,
			entry.MetricLabelBackend:    backend.Service,
		}).Set(float64(ts.Spec.Backends[i].Weight))
	}

	if err := r.Update(ctx, ts); err != nil {
		return fmt.Errorf("failed to update TrafficSplit: %w", err)
	}
	return nil
}

func (r *TrafficSplitReconciler) isControlled(ts *trafficsplitv1alpha2.TrafficSplit) bool {
	v, ok := ts.GetLabels()[LabelControlledByKey]
	if !ok {
		return false
	}
	return v == r.LabelControlledByValue
}
