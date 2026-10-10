/*
Copyright 2026.

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

package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

// CustomDeploymentReconciler reconciles a CustomDeployment object
type CustomDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customdeployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customdeployments/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customdeployments/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile keeps a real Deployment matching the CustomDeployment spec in the
// namespace named by the referenced CustomNamespace. Deployments not carrying
// this operator's owner label are never touched (NameConflict instead).
func (r *CustomDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cd customv1.CustomDeployment
	if err := r.Get(ctx, req.NamespacedName, &cd); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cd.DeletionTimestamp.IsZero() {
		var d appsv1.Deployment
		err := r.Get(ctx, client.ObjectKey{Namespace: cd.Spec.Namespace, Name: cd.Spec.Name}, &d)
		if apierrors.IsNotFound(err) {
			controllerutil.RemoveFinalizer(&cd, customv1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, &cd)
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if d.Labels[customv1.OwnerLabelKey] != ownerRef("CustomDeployment", cd.Namespace, cd.Name) {
			// not ours: nothing to clean up
			controllerutil.RemoveFinalizer(&cd, customv1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, &cd)
		}
		if err := r.Delete(ctx, &d); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// gate on the referenced CustomNamespace existing
	var cn customv1.CustomNamespace
	if err := r.Get(ctx, client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.Namespace}, &cn); apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "MissingCustomNamespace",
			Message: "CustomNamespace " + cd.Spec.Namespace + " does not exist; retrying until it appears",
		})
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if err := ensureFinalizer(ctx, r.Client, &cd); err != nil {
		return ctrl.Result{}, err
	}

	replicas := int32(1)
	if cd.Spec.Replicas != nil {
		replicas = *cd.Spec.Replicas
	}
	owner := ownerRef("CustomDeployment", cd.Namespace, cd.Name)

	var d appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Namespace: cd.Spec.Namespace, Name: cd.Spec.Name}, &d)
	switch {
	case apierrors.IsNotFound(err):
		d = appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cd.Spec.Name,
				Namespace: cd.Spec.Namespace,
				Labels:    map[string]string{customv1.OwnerLabelKey: owner},
			},
			Spec: deploymentSpec(cd.Spec.Name, cd.Spec.Image, replicas),
		}
		if err := r.Create(ctx, &d); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
		log.Info("created deployment", "deployment", cd.Spec.Name, "namespace", cd.Spec.Namespace)
	case err != nil:
		return ctrl.Result{}, err
	case d.Labels[customv1.OwnerLabelKey] != owner:
		log.Info("name taken by an unmanaged deployment", "deployment", cd.Spec.Name, "namespace", cd.Spec.Namespace)
		return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NameConflict",
			Message: "deployment " + cd.Spec.Name + " already exists in namespace " + cd.Spec.Namespace + " and is not managed by this operator",
		})
	default:
		// ours: repair drift in the fields we own
		if d.Spec.Template.Spec.Containers[0].Image != cd.Spec.Image || d.Spec.Replicas == nil || *d.Spec.Replicas != replicas {
			d.Spec.Replicas = &replicas
			d.Spec.Template.Spec.Containers[0].Image = cd.Spec.Image
			if err := r.Update(ctx, &d); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("repaired deployment drift", "deployment", cd.Spec.Name, "namespace", cd.Spec.Namespace)
		}
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
		Type:    customv1.ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "deployment is present and matches the spec",
	})
}

// deploymentSpec builds the single-container Deployment spec the operator owns.
func deploymentSpec(name, image string, replicas int32) appsv1.DeploymentSpec {
	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: image}}},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&customv1.CustomDeployment{}).
		Named("customdeployment").
		Complete(r)
}
