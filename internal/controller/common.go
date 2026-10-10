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

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

// reasonReady is the success Reason reported by every controller.
const reasonReady = "Ready"

// setCondition writes cond into conditions on obj unless an equal condition is
// already set; returns the status-write error only.
func setCondition(ctx context.Context, w client.StatusWriter, obj client.Object, conditions *[]metav1.Condition, cond metav1.Condition) error {
	cond.ObservedGeneration = obj.GetGeneration()
	if !meta.SetStatusCondition(conditions, cond) {
		return nil
	}
	return w.Update(ctx, obj)
}

// ownerRef returns the owner-identity value stamped on real resources.
// Label values forbid '/', so the parts are joined with '.'.
func ownerRef(kind, namespace, name string) string {
	return kind + "." + namespace + "." + name
}

// ensureFinalizer adds the shared finalizer to obj, persisting it if added.
func ensureFinalizer(ctx context.Context, c client.Client, obj client.Object) error {
	if !controllerutil.AddFinalizer(obj, customv1.FinalizerName) {
		return nil
	}
	return c.Update(ctx, obj)
}
