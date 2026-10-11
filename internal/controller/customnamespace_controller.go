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
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

// requeueInterval is how often every controller re-checks its real resources.
// shortcut: drift repair and deletion progress can lag up to this interval; watch the real resources instead when immediate reaction is required.
const requeueInterval = 30 * time.Second

// CustomNamespaceReconciler reconciles a CustomNamespace object
type CustomNamespaceReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Discovery discovery.DiscoveryInterface
}

// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customnamespaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customnamespaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=custom.jurikolo.name,resources=customnamespaces/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;delete
// read-only cluster-wide: needed to check whether a namespace still holds any
// resources before allowing CustomNamespace deletion
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list

// Reconcile keeps a real namespace named by the object's spec.name.
// Deleting the object is blocked (NamespaceDraining) while the namespace
// still contains any resource; only an empty namespace is deleted.
// Namespaces not carrying this operator's owner label are never touched
// (NameConflict instead).
func (r *CustomNamespaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var cn customv1.CustomNamespace
	if err := r.Get(ctx, req.NamespacedName, &cn); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "get failed", "CustomNamespace", req.Namespace+"/"+req.Name)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	owner := ownerRef("CustomNamespace", cn.Namespace, cn.Name)

	if !cn.DeletionTimestamp.IsZero() {
		var ns corev1.Namespace
		err := r.Get(ctx, client.ObjectKey{Name: cn.Spec.Name}, &ns)
		if apierrors.IsNotFound(err) {
			controllerutil.RemoveFinalizer(&cn, customv1.FinalizerName)
			if err := r.Update(ctx, &cn); err != nil {
				log.Error(err, "update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if err != nil {
			log.Error(err, "get failed", "namespace", cn.Spec.Name)
			return ctrl.Result{}, err
		}
		if ns.Labels[customv1.OwnerLabelKey] != owner {
			// not ours: nothing to clean up
			controllerutil.RemoveFinalizer(&cn, customv1.FinalizerName)
			if err := r.Update(ctx, &cn); err != nil {
				log.Error(err, "update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		empty, err := r.namespaceEmpty(ctx, cn.Spec.Name)
		if err != nil {
			log.Error(err, "list failed", "namespace", cn.Spec.Name)
			return ctrl.Result{}, err
		}
		if !empty {
			log.Info("deletion blocked: namespace still contains resources", "namespace", cn.Spec.Name)
			err := setCondition(ctx, r.Status(), &cn, &cn.Status.Conditions, metav1.Condition{
				Type:    customv1.ConditionTypeReady,
				Status:  metav1.ConditionFalse,
				Reason:  "NamespaceDraining",
				Message: "deletion blocked: namespace still contains resources; empty it to proceed",
			})
			if err != nil {
				log.Error(err, "status update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
			}
			return ctrl.Result{RequeueAfter: requeueInterval}, err
		}
		if err := r.Delete(ctx, &ns); client.IgnoreNotFound(err) != nil {
			log.Error(err, "delete failed", "namespace", cn.Spec.Name)
			return ctrl.Result{}, err
		}
		log.Info("deleted namespace", "namespace", cn.Spec.Name, "CustomNamespace", cn.Namespace+"/"+cn.Name)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	if err := ensureFinalizer(ctx, r.Client, &cn); err != nil {
		log.Error(err, "update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
		return ctrl.Result{}, err
	}

	var ns corev1.Namespace
	err := r.Get(ctx, client.ObjectKey{Name: cn.Spec.Name}, &ns)
	switch {
	case apierrors.IsNotFound(err):
		if err := r.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{
			Name:   cn.Spec.Name,
			Labels: map[string]string{customv1.OwnerLabelKey: owner},
		}}); err != nil && !apierrors.IsAlreadyExists(err) {
			// report the failure on the object, then return it for backoff retry
			log.Error(err, "create failed", "namespace", cn.Spec.Name)
			_ = setCondition(ctx, r.Status(), &cn, &cn.Status.Conditions, metav1.Condition{
				Type:    customv1.ConditionTypeReady,
				Status:  metav1.ConditionFalse,
				Reason:  reasonCreateFailed,
				Message: err.Error(),
			})
			return ctrl.Result{}, err
		}
		log.Info("created namespace", "namespace", cn.Spec.Name, "CustomNamespace", cn.Namespace+"/"+cn.Name)
	case err != nil:
		log.Error(err, "get failed", "namespace", cn.Spec.Name)
		return ctrl.Result{}, err
	case !ns.DeletionTimestamp.IsZero():
		// old namespace still terminating; recreate once it is gone
		err := setCondition(ctx, r.Status(), &cn, &cn.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  "Reconciling",
			Message: "previous namespace is terminating; a new one will be created",
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	case ns.Labels[customv1.OwnerLabelKey] != owner:
		existing := ns.Labels[customv1.OwnerLabelKey]
		if existing == "" {
			existing = "no one (unmanaged)"
		}
		log.Info("namespace already claimed", "namespace", cn.Spec.Name, "owner", existing)
		err := setCondition(ctx, r.Status(), &cn, &cn.Status.Conditions, metav1.Condition{
			Type:    customv1.ConditionTypeReady,
			Status:  metav1.ConditionFalse,
			Reason:  reasonNameConflict,
			Message: "namespace " + cn.Spec.Name + " is already claimed; existing owner: " + existing,
		})
		if err != nil {
			log.Error(err, "status update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
		}
		return ctrl.Result{RequeueAfter: requeueInterval}, err
	}

	err = setCondition(ctx, r.Status(), &cn, &cn.Status.Conditions, metav1.Condition{
		Type:    customv1.ConditionTypeReady,
		Status:  metav1.ConditionTrue,
		Reason:  reasonReady,
		Message: "namespace is present",
	})
	if err != nil {
		log.Error(err, "status update failed", "CustomNamespace", cn.Namespace+"/"+cn.Name)
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, err
}

// namespaceEmpty reports whether the namespace holds no resources. It lists
// every namespaced kind the API server prefers, ignoring the objects
// Kubernetes injects into every namespace (events, kube-root-ca.crt).
// shortcut: kinds not readable with the operator's RBAC are invisible to this check; tighten RBAC if blind spots matter.
func (r *CustomNamespaceReconciler) namespaceEmpty(ctx context.Context, name string) (bool, error) {
	apiLists, err := r.Discovery.ServerPreferredNamespacedResources()
	if err != nil {
		return false, err
	}
	for _, apiList := range apiLists {
		gv, err := schema.ParseGroupVersion(apiList.GroupVersion)
		if err != nil {
			continue
		}
		for _, res := range apiList.APIResources {
			if strings.Contains(res.Name, "/") || !slices.Contains(res.Verbs, "list") {
				continue // subresources and non-listable kinds
			}
			gvk := schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: res.Kind}
			if gvk.Kind == "Event" {
				continue
			}
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{Group: gv.Group, Version: gv.Version, Kind: res.Kind + "List"})
			if err := r.List(ctx, list, client.InNamespace(name)); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return false, err
			}
			for _, item := range list.Items {
				if isInjected(gvk, item.GetName()) {
					continue
				}
				return false, nil
			}
		}
	}
	return true, nil
}

// isInjected reports whether an object is one Kubernetes creates in every
// namespace on its own; such objects do not count as user contents.
func isInjected(gvk schema.GroupVersionKind, name string) bool {
	return (gvk.Group == "" && gvk.Kind == "ConfigMap" && name == "kube-root-ca.crt") ||
		(gvk.Group == "" && gvk.Kind == "ServiceAccount" && name == "default")
}

// SetupWithManager sets up the controller with the Manager.
func (r *CustomNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	r.Discovery = discoveryClient
	return ctrl.NewControllerManagedBy(mgr).
		For(&customv1.CustomNamespace{}).
		Named("customnamespace").
		Complete(r)
}
