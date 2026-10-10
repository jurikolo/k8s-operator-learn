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
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cc.DeletionTimestamp.IsZero() {
		var cm corev1.ConfigMap
		err := r.Get(ctx, client.ObjectKey{Namespace: cc.Spec.Namespace, Name: cc.Name}, &cm)
		if apierrors.IsNotFound(err) {
			controllerutil.RemoveFinalizer(&cc, customv1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, &cc)
		}
		if err != nil {
			return ctrl.Result{}, err
		}
		if cm.Labels[customv1.OwnerLabelKey] != ownerRef("CustomConfig", cc.Namespace, cc.Name) {
			// not ours: nothing to clean up
			controllerutil.RemoveFinalizer(&cc, customv1.FinalizerName)
			return ctrl.Result{}, r.Update(ctx, &cc)
		}
		if err := r.Delete(ctx, &cm); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// gate on the referenced CustomNamespace existing
	var cn customv1.CustomNamespace
	if err := r.Get(ctx, client.ObjectKey{Namespace: cc.Namespace, Name: cc.Spec.Namespace}, &cn); apierrors.IsNotFound(err) {
		return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "MissingCustomNamespace",
			Message: "CustomNamespace " + cc.Spec.Namespace + " does not exist; retrying until it appears",
		})
	} else if err != nil {
		return ctrl.Result{}, err
	}

	if err := ensureFinalizer(ctx, r.Client, &cc); err != nil {
		return ctrl.Result{}, err
	}

	owner := ownerRef("CustomConfig", cc.Namespace, cc.Name)

	var cm corev1.ConfigMap
	err := r.Get(ctx, client.ObjectKey{Namespace: cc.Spec.Namespace, Name: cc.Name}, &cm)
	switch {
	case apierrors.IsNotFound(err):
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cc.Name,
				Namespace: cc.Spec.Namespace,
				Labels:    map[string]string{customv1.OwnerLabelKey: owner},
			},
			Data:       cc.Spec.Data,
			BinaryData: cc.Spec.BinaryData,
			Immutable:  cc.Spec.Immutable,
		}
		if err := r.Create(ctx, &cm); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}
		log.Info("created configmap", "configmap", cc.Name, "namespace", cc.Spec.Namespace)
	case err != nil:
		return ctrl.Result{}, err
	case cm.Labels[customv1.OwnerLabelKey] != owner:
		log.Info("name taken by an unmanaged configmap", "configmap", cc.Name, "namespace", cc.Spec.Namespace)
		return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "NameConflict",
			Message: "configmap " + cc.Name + " already exists in namespace " + cc.Spec.Namespace + " and is not managed by this operator",
		})
	default:
		// ours: repair content drift
		if !reflect.DeepEqual(cm.Data, cc.Spec.Data) || !reflect.DeepEqual(cm.BinaryData, cc.Spec.BinaryData) ||
			!reflect.DeepEqual(cm.Immutable, cc.Spec.Immutable) {
			cm.Data = cc.Spec.Data
			cm.BinaryData = cc.Spec.BinaryData
			cm.Immutable = cc.Spec.Immutable
			if err := r.Update(ctx, &cm); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("repaired configmap drift", "configmap", cc.Name, "namespace", cc.Spec.Namespace)
		}
	}

	return ctrl.Result{RequeueAfter: requeueInterval}, setCondition(ctx, r.Status(), &cc, &cc.Status.Conditions, metav1.Condition{
		Type:    customv1.ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "configmap is present and matches the spec",
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&customv1.CustomConfig{}).
		Named("customconfig").
		Complete(r)
}
