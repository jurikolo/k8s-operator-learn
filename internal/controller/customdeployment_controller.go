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
		if !apierrors.IsNotFound(err) {
			log.Error(err, "get failed", "CustomDeployment", req.Namespace+"/"+req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cd.DeletionTimestamp.IsZero() {
		// the real resource lives in the namespace named by the referenced
		// CustomNamespace's spec.name; resolve the reference first
		var cn customv1.CustomNamespace
		err := r.Get(ctx, client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.Namespace}, &cn)
		if apierrors.IsNotFound(err) {
			// reference gone: the real resource is unreachable; release the finalizer
			controllerutil.RemoveFinalizer(&cd, customv1.FinalizerName)
			if err := r.Update(ctx, &cd); err != nil {
				log.Error(err, "update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			log.Error(err, "get failed", "CustomNamespace", cd.Spec.Namespace)
			return ctrl.Result{}, err
		}
		realNS := cn.Spec.Name
		var d appsv1.Deployment
		err = r.Get(ctx, client.ObjectKey{Namespace: realNS, Name: cd.Spec.Name}, &d)
		if apierrors.IsNotFound(err) {
			controllerutil.RemoveFinalizer(&cd, customv1.FinalizerName)
			if err := r.Update(ctx, &cd); err != nil {
				log.Error(err, "update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			log.Error(err, "get failed", "deployment", cd.Spec.Name, "namespace", realNS)
			return ctrl.Result{}, err
		}
		if d.Labels[customv1.OwnerLabelKey] != ownerRef("CustomDeployment", cd.Namespace, cd.Name) {
			// not ours: nothing to clean up
			controllerutil.RemoveFinalizer(&cd, customv1.FinalizerName)
			if err := r.Update(ctx, &cd); err != nil {
				log.Error(err, "update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err := r.Delete(ctx, &d); client.IgnoreNotFound(err) != nil {
			log.Error(err, "delete failed", "deployment", cd.Spec.Name, "namespace", realNS)
			return ctrl.Result{}, err
		}
		log.Info("deleted deployment", "deployment", cd.Spec.Name, "namespace", realNS, "CustomDeployment", cd.Namespace+"/"+cd.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// gate on the referenced CustomNamespace existing
	var cn customv1.CustomNamespace
	if err := r.Get(ctx, client.ObjectKey{Namespace: cd.Namespace, Name: cd.Spec.Namespace}, &cn); apierrors.IsNotFound(err) {
		err := setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "MissingCustomNamespace",
			Message: "CustomNamespace " + cd.Spec.Namespace + " does not exist; retrying until it appears",
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	} else if err != nil {
		log.Error(err, "get failed", "CustomNamespace", cd.Spec.Namespace)
		return ctrl.Result{}, err
	}

	if err := ensureFinalizer(ctx, r.Client, &cd); err != nil {
		log.Error(err, "update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
		return ctrl.Result{}, err
	}

	replicas := int32(1)
	if cd.Spec.Replicas != nil {
		replicas = *cd.Spec.Replicas
	}
	owner := ownerRef("CustomDeployment", cd.Namespace, cd.Name)
	// the Deployment lives in the real namespace the referenced CustomNamespace manages
	realNS := cn.Spec.Name

	var d appsv1.Deployment
	err := r.Get(ctx, client.ObjectKey{Namespace: realNS, Name: cd.Spec.Name}, &d)
	switch {
	case apierrors.IsNotFound(err):
		d = appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cd.Spec.Name,
				Namespace: realNS,
				Labels:    map[string]string{customv1.OwnerLabelKey: owner},
			},
			Spec: deploymentSpec(cd.Spec.Name, cd.Spec.Image, replicas),
		}
		if err := r.Create(ctx, &d); err != nil && !apierrors.IsAlreadyExists(err) {
			// report the failure on the object, then return it for backoff retry
			log.Error(err, "create failed", "deployment", cd.Spec.Name, "namespace", realNS)
			_ = setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
				Type:    customv1.ConditionTypeReady,
				Status:  metav1.ConditionFalse,
				Reason:  reasonCreateFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
		log.Info("created deployment", "deployment", cd.Spec.Name, "namespace", realNS, "CustomDeployment", cd.Namespace+"/"+cd.Name)
	case err != nil:
		log.Error(err, "get failed", "deployment", cd.Spec.Name, "namespace", realNS)
		return ctrl.Result{}, err
	case d.Labels[customv1.OwnerLabelKey] != owner:
		log.Info("name taken by an unmanaged deployment", "deployment", cd.Spec.Name, "namespace", realNS)
		err := setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  reasonNameConflict,
			Message: "deployment " + cd.Spec.Name + " already exists in namespace " + realNS + " and is not managed by this operator",
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	default:
		// ours: repair drift in the fields we own
		if d.Spec.Template.Spec.Containers[0].Image != cd.Spec.Image || d.Spec.Replicas == nil || *d.Spec.Replicas != replicas {
			d.Spec.Replicas = &replicas
			d.Spec.Template.Spec.Containers[0].Image = cd.Spec.Image
			if err := r.Update(ctx, &d); err != nil {
				log.Error(err, "update failed", "deployment", cd.Spec.Name, "namespace", realNS)
				return ctrl.Result{}, err
			}
			log.Info("repaired deployment drift", "deployment", cd.Spec.Name, "namespace", realNS)
		}
	}

	err = setCondition(ctx, r.Status(), &cd, &cd.Status.Conditions, metav1.Condition{
		Type:    customv1.ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "deployment is present and matches the spec",
	})
	if err != nil {
		log.Error(err, "status update failed", "CustomDeployment", cd.Namespace+"/"+cd.Name)
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, err
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
