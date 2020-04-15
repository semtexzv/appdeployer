/*
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
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	//deployv1alpha1 "semtexzv.com/appdeployer/api/v1alpha1"
	"semtexzv.com/appdeployer/controllers"

	osappsv1 "github.com/openshift/api/apps/v1"
	osbuildv1 "github.com/openshift/api/build/v1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	// TODO: Add support for CRD
	//_ = deployv1alpha1.AddToScheme(scheme)

	_ = osappsv1.AddToScheme(scheme)
	_ = osbuildv1.AddToScheme(scheme)

	// +kubebuilder:scaffold:scheme
}

func main() {
	var namespace string
	var metricsAddr string
	var enableLeaderElection bool

	var cfgMap string
	var cfgMapKey string
	flag.StringVar(&cfgMap, "configmap", "", "Name of configmap to watch for versions")
	flag.StringVar(&cfgMapKey, "configkey", "", "key within the configmap to watch")
	flag.StringVar(&namespace, "namespace", "default", "namespace, which the controller is supposed to watch")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		Namespace:          namespace,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "f905d61a.semtexzv.com",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.AppDeploymentReconciler{
		Client:     mgr.GetClient(),
		Log:        ctrl.Log.WithName("controllers").WithName("AppDeployment"),
		Scheme:     mgr.GetScheme(),
		CfgMapName: cfgMap,
		CfgMapKey:  cfgMapKey,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "AppDeployment")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
