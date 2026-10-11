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
	"reflect"

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

// CustomConfigReconciler reconciles a CustomConfig object
type CustomConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile keeps a real ConfigMap with the CustomConfig's content in the
// namespace named by the referenced CustomNamespace. ConfigMaps not carrying
// this operator's owner label are never touched (NameConflict instead).
func (r *CustomConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cc customv1.CustomConfig
	if err := r.Get(ctx, req.NamespacedName, &cc); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "get failed", "CustomConfig", req.Namespace+"/"+req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cc.DeletionTimestamp.IsZero() {
		// the real resource lives in the namespace named by the referenced
		// CustomNamespace's spec.name; resolve the reference first
		var cn customv1.CustomNamespace
		err := r.Get(ctx, client.ObjectKey{Namespace: cc.Namespace, Name: cc.Spec.Namespace}, &cn)
		if apierrors.IsNotFound(err) {
			// reference gone: the real resource is unreachable; release the finalizer
			controllerutil.RemoveFinalizer(&cc, customv1.FinalizerName)
			if err := r.Update(ctx, &cc); err != nil {
				log.Error(err, "update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			log.Error(err, "get failed", "CustomNamespace", cc.Spec.Namespace)
			return ctrl.Result{}, err
		}
		realNS := cn.Spec.Name
		var cm corev1.ConfigMap
		err = r.Get(ctx, client.ObjectKey{Namespace: realNS, Name: cc.Spec.Name}, &cm)
		if apierrors.IsNotFound(err) {
			controllerutil.RemoveFinalizer(&cc, customv1.FinalizerName)
			if err := r.Update(ctx, &cc); err != nil {
				log.Error(err, "update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			log.Error(err, "get failed", "configmap", cc.Spec.Name, "namespace", realNS)
			return ctrl.Result{}, err
		}
		if cm.Labels[customv1.OwnerLabelKey] != ownerRef("CustomConfig", cc.Namespace, cc.Name) {
			// not ours: nothing to clean up
			controllerutil.RemoveFinalizer(&cc, customv1.FinalizerName)
			if err := r.Update(ctx, &cc); err != nil {
				log.Error(err, "update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err := r.Delete(ctx, &cm); client.IgnoreNotFound(err) != nil {
			log.Error(err, "delete failed", "configmap", cc.Spec.Name, "namespace", realNS)
			return ctrl.Result{}, err
		}
		log.Info("deleted configmap", "configmap", cc.Spec.Name, "namespace", realNS, "CustomConfig", cc.Namespace+"/"+cc.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// gate on the referenced CustomNamespace existing
	var cn customv1.CustomNamespace
	if err := r.Get(ctx, client.ObjectKey{Namespace: cc.Namespace, Name: cc.Spec.Namespace}, &cn); apierrors.IsNotFound(err) {
		err := setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "MissingCustomNamespace",
			Message: "CustomNamespace " + cc.Spec.Namespace + " does not exist; retrying until it appears",
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	} else if err != nil {
		log.Error(err, "get failed", "CustomNamespace", cc.Spec.Namespace)
		return ctrl.Result{}, err
	}

	if err := ensureFinalizer(ctx, r.Client, &cc); err != nil {
		log.Error(err, "update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
		return ctrl.Result{}, err
	}

	owner := ownerRef("CustomConfig", cc.Namespace, cc.Name)
	// the ConfigMap lives in the real namespace the referenced CustomNamespace manages
	realNS := cn.Spec.Name

	var cm corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{Namespace: realNS, Name: cc.Spec.Name}, &cm)
	switch {
	case apierrors.IsNotFound(err):
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cc.Spec.Name,
				Namespace: realNS,
				Labels:    map[string]string{customv1.OwnerLabelKey: owner},
			},
			Data:       cc.Spec.Data,
			BinaryData: cc.Spec.BinaryData,
			Immutable:  cc.Spec.Immutable,
		}
		if err := r.Create(ctx, &cm); err != nil && !apierrors.IsAlreadyExists(err) {
			// report the failure on the object, then return it for backoff retry
			log.Error(err, "create failed", "configmap", cc.Spec.Name, "namespace", realNS)
			_ = setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
				Type:    customv1.ConditionTypeReady,
				Status:  metav1.ConditionFalse,
				Reason:  reasonCreateFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
		log.Info("created configmap", "configmap", cc.Spec.Name, "namespace", realNS, "CustomConfig", cc.Namespace+"/"+cc.Name)
	case err != nil:
		log.Error(err, "get failed", "configmap", cc.Spec.Name, "namespace", realNS)
		return ctrl.Result{}, err
	case cm.Labels[customv1.OwnerLabelKey] != owner:
		log.Info("name taken by an unmanaged configmap", "configmap", cc.Spec.Name, "namespace", realNS)
		err := setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  reasonNameConflict,
			Message: "configmap " + cc.Spec.Name + " already exists in namespace " + realNS + " and is not managed by this operator",
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	default:
		// ours: repair content drift
		if !reflect.DeepEqual(cm.Data, cc.Spec.Data) || !reflect.DeepEqual(cm.BinaryData, cc.Spec.BinaryData) ||
			!reflect.DeepEqual(cm.Immutable, cc.Spec.Immutable) {
			cm.Data = cc.Spec.Data
			cm.BinaryData = cc.Spec.BinaryData
			cm.Immutable = cc.Spec.Immutable
			if err := r.Update(ctx, &cm); err != nil {
				log.Error(err, "update failed", "configmap", cc.Spec.Name, "namespace", realNS)
				return ctrl.Result{}, err
			}
			log.Info("repaired configmap drift", "configmap", cc.Spec.Name, "namespace", realNS)
		}
	}

	err = setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
		Type:    customv1.ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "configmap is present and matches the spec",
	})
	if err != nil {
		log.Error(err, "status update failed", "CustomConfig", cc.Namespace+"/"+cc.Name)
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&customv1.CustomConfig{}).
		Named("customconfig").
		Complete(r)
}
