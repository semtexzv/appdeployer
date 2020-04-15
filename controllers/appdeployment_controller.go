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

package controllers

import (
	"context"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
	"strings"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	appsv1 "github.com/openshift/api/apps/v1"
	buildv1 "github.com/openshift/api/build/v1"
)

var True = true

// AppDeploymentReconciler reconciles a AppDeployment object
type AppDeploymentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	CfgMapName string
	CfgMapKey  string
}

// +kubebuilder:rbac:groups=deploy.semtexzv.com,resources=appdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=deploy.semtexzv.com,resources=appdeployments/status,verbs=get;update;patch

func (r *AppDeploymentReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("appdeployment", req.NamespacedName)
	log.Info("Reconcile")

	var desiredVersion string

	var config v1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
		log.Error(err, "Unable to retrieve configmap")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if config.Name != r.CfgMapName {
		return ctrl.Result{}, nil
	}
	if ver, has := config.Data[r.CfgMapKey]; has {
		desiredVersion = ver
		log.Info("Desired version is ", "ver", desiredVersion)
	}

	/*
		var config deployv1alpha1.AppDeployment
		if err := r.Get(ctx, req.NamespacedName, &config); err != nil {
			log.Error(err, "Unable to fetch AppDeployment")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	*/

	var buildConfigs buildv1.BuildConfigList
	if err := r.List(ctx, &buildConfigs, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "Unable to retrieve BuildConfigs")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, v := range buildConfigs.Items {
		targetParts := strings.Split(v.Spec.Output.To.Name, ":")
		if v.Spec.Source.Type != "Git" {
			continue
		}

		// If we don't have the output set to correct tag, change it and push changes
		if targetParts[1] == desiredVersion {
			continue
		}

		v.Spec.Output.To.Name = strings.Join([]string{targetParts[0], desiredVersion}, ":")
		v.Spec.Source.Git.Ref = desiredVersion
		v.Status.LastVersion += 1
		if err := r.Update(ctx, &v); err != nil {
			log.Error(err, "Unable to change BuildConfig to conform to latest AppDeployment version")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		buildNumber := strconv.Itoa(int(v.Status.LastVersion))
		buildName := v.Name + "-" + buildNumber

		labels := v.Labels
		labels[buildv1.BuildConfigLabelDeprecated] = v.Name
		labels[buildv1.BuildConfigLabel] = v.Name
		labels[buildv1.BuildRunPolicyLabel] = string(buildv1.BuildRunPolicySerialLatestOnly)

		annotations := map[string]string{}
		annotations[buildv1.BuildConfigAnnotation] = v.Name
		annotations[buildv1.BuildNumberAnnotation] = buildNumber
		annotations[buildv1.BuildPodNameAnnotation] = buildName + "-build"

		if err := r.Create(ctx, &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Name:        buildName,
				Namespace:   v.Namespace,
				Labels:      labels,
				Annotations: annotations,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: v.APIVersion,
					Kind:       v.Kind,
					Name:       v.Name,
					UID:        v.UID,
					Controller: &True,
				},
				},
			},
			Spec: buildv1.BuildSpec{
				CommonSpec:  v.Spec.CommonSpec,
				TriggeredBy: []buildv1.BuildTriggerCause{{Message: "AppDeployer"}},
			},
		}); err != nil {
			log.Error(err, "Unable to create new build")
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

	}

	var deployConfigs appsv1.DeploymentConfigList
	if err := r.List(ctx, &deployConfigs, client.InNamespace(req.Namespace)); err != nil {
		log.Error(err, "Unable to retrieve DeploymentConfigs")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	for _, v := range deployConfigs.Items {
		for _, trig := range v.Spec.Triggers {
			if trig.Type == appsv1.DeploymentTriggerOnImageChange {
				parts := strings.Split(trig.ImageChangeParams.From.Name, ":")
				if parts[1] == desiredVersion {
					continue
				}
				trig.ImageChangeParams.From.Name = strings.Join([]string{parts[0], desiredVersion}, ":")
				if err := r.Update(ctx, &v); err != nil {
					log.Error(err, "Unable to change BuildConfig to conform to latest AppDeployment desiredVersion")
					return ctrl.Result{}, client.IgnoreNotFound(err)
				}
			}
		}

	}

	return ctrl.Result{}, nil
}

func (r *AppDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// TODO: Integrate imagestreams & tags correctly
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1.ConfigMap{}).
		Owns(&v1.ConfigMap{}).
		// TODO: Add support for CRD
		//For(&deployv1alpha1.AppDeployment{}).
		Owns(&appsv1.DeploymentConfig{}).
		Owns(&buildv1.BuildConfig{}).
		Complete(r)
}
