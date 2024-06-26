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

package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	configv2 "github.com/oliviermichaelis/l3/api/v2"
	"github.com/oliviermichaelis/l3/controllers"
	"github.com/oliviermichaelis/l3/pkg/entry"
	"github.com/oliviermichaelis/l3/pkg/ewma"
	"github.com/oliviermichaelis/l3/pkg/manager"
	"github.com/oliviermichaelis/l3/pkg/metrics"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	trafficsplitv1alpha2 "github.com/servicemeshinterface/smi-sdk-go/pkg/apis/split/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/linkerd/linkerd2/pkg/k8s"
	"github.com/linkerd/linkerd2/viz/metrics-api/client"
	"github.com/linkerd/linkerd2/viz/metrics-api/gen/viz"

	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(trafficsplitv1alpha2.AddToScheme(scheme))
	utilruntime.Must(configv2.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr                      string
		enableLeaderElection             bool
		probeAddr                        string
		controlledBy                     string
		linkerdVizNamespace              string
		configFile                       string
		enabledDebug                     bool
		enabledRequestRateControl        bool
		exponentialWeightedMovingAverage string
		weightingAlgorithm               string
		failureLatencySeconds            float64
		linkerdMetricsTimeWindow         string
	)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection,
		"leader-elect",
		false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&controlledBy, "controlled-by", "", fmt.Sprintf("The %s selector value used to determine ownership.", controllers.LabelControlledByKey))
	flag.StringVar(&linkerdVizNamespace, "linkerd-viz-namespace", "linkerd-viz", "The namespace in which linkerd-viz is installed.")
	flag.StringVar(&configFile, "config-file", "", "The path to the config file.")
	flag.BoolVar(&enabledDebug, "debug", false, "Enable debug mode")
	flag.BoolVar(&enabledRequestRateControl, "rate-control", true, "Rate control to react to RPS changes")
	flag.StringVar(&exponentialWeightedMovingAverage, "ewma-algorithm", "ewma", "The EWMA algorithm to use")
	flag.StringVar(&weightingAlgorithm, "weighting-algorithm", "l3", "The weighting algorithm to use")
	flag.Float64Var(&failureLatencySeconds, "failure-latency-seconds", 0.6, "The penalty factor for unsuccessful responses")
	flag.StringVar(&linkerdMetricsTimeWindow, "linkerd-metrics-time-window", "10s", "The time window for aggregated metrics")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Validate flags
	if controlledBy == "" {
		setupLog.Error(errors.New("controlled-by flag not set"), "")
		os.Exit(1)
	}

	if exponentialWeightedMovingAverage != string(ewma.EWMAType) && exponentialWeightedMovingAverage != string(ewma.PeakEWMAType) {
		setupLog.Error(errors.New("only \"ewma\" or \"peakewma\" are valid values for ewma-algorithm flag"), "")
		os.Exit(1)
	}

	// Set the package-level variable according to the flag value
	entry.RequestRateControlEnabled = enabledRequestRateControl

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5686836a.oliviermichaelis.dev",
	}
	var err error
	ctrlConfig := configv2.ProjectConfig{}
	if configFile != "" {
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&ctrlConfig))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	/*
		The metrics-api ServerAuthorization needs to be adjusted. Otherwise, a 403 is returned: https://linkerd.io/2.11/reference/authorization-policy/
		spec:
		  client:
		    meshTLS:
		      serviceAccounts:
		      - name: web
		      - name: prometheus
		      - name: linkerd-least-latency-controller-manager
		        namespace: linkerd-least-latency-system
	*/
	ctx := ctrl.SetupSignalHandler()
	var prometheusAddress string
	var metricsClient viz.ApiClient
	if enabledDebug {
		prometheusAddress = "http://localhost:9090"
		k8sApi, err := k8s.NewAPI("", "", "", nil, 5*time.Second)
		if err != nil {
			setupLog.Error(err, "unable to create k8s debug client")
			os.Exit(1)
		}
		metricsClient, err = client.NewExternalClient(ctx, "linkerd-viz", k8sApi)
	} else {
		prometheusAddress = "http://prometheus.linkerd-viz.svc.cluster.local:9090"
		metricsClient, err = client.NewInternalClient(linkerdVizNamespace,
			fmt.Sprintf("%s.%s.svc.cluster.local:%d",
				ctrlConfig.MetricsApi.ServiceName,
				ctrlConfig.MetricsApi.Namespace,
				ctrlConfig.MetricsApi.Port))
	}
	if err != nil {
		setupLog.Error(err, "unable to create metrics API client")
		os.Exit(1)
	}
	linkerd := metrics.NewLinkerdMetrics(metricsClient, linkerdMetricsTimeWindow)
	reconciler := &controllers.TrafficSplitReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		LabelControlledByValue: controlledBy,
	}
	prometheus, err := metrics.NewPrometheusMetrics(prometheusAddress)
	if err != nil {
		setupLog.Error(err, "unable to create prometheus client")
		os.Exit(1)
	}

	var algorithm entry.Algorithm
	switch weightingAlgorithm {
	case "l3":
		algorithm = entry.NewInFlightLatency(failureLatencySeconds)
	case "c3":
		algorithm = &entry.C3{}
	case "l3v2":
		algorithm = entry.NewL3(failureLatencySeconds)
	default:
		setupLog.Error(nil, fmt.Sprintf("unknown weighting algorithm: %s", weightingAlgorithm))
		os.Exit(1)
	}

	m := manager.NewManager(ctx, linkerd, prometheus, reconciler, algorithm, ewma.TypeEWMA(exponentialWeightedMovingAverage))
	reconciler.Manager = m
	if err = (reconciler).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrafficSplit")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
