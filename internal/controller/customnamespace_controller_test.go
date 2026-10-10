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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

var _ = Describe("CustomNamespace Controller", func() {
	ctx := context.Background()

	Describe("admission validation", func() {
		It("rejects reserved namespace names", func() {
			for _, name := range []string{"kube-system", "default", "kube-foo"} {
				err := k8sClient.Create(ctx, &customv1.CustomNamespace{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
					Spec:       customv1.CustomNamespaceSpec{},
				})
				Expect(err).To(MatchError(ContainSubstring("reserved")), "name: %s", name)
			}
		})
	})

	Describe("reconciliation", func() {
		It("creates the real namespace and reports Ready", func() {
			createCN(ctx, "us1-team-a")
			reconcileCN(ctx, "us1-team-a")

			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-a"}, ns)).To(Succeed())

			cn := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-a", Namespace: "default"}, cn)).To(Succeed())
			ready := metaFindCondition(cn.Status.Conditions)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal("Ready"))
		})

		It("recreates the namespace after out-of-band deletion", func() {
			createCN(ctx, "us1-team-b")
			reconcileCN(ctx, "us1-team-b")
			hardDeleteNamespace(ctx, "us1-team-b")

			reconcileCN(ctx, "us1-team-b")
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-b"}, &corev1.Namespace{})).To(Succeed())
		})

		It("blocks deletion while the namespace contains resources", func() {
			createCN(ctx, "us1-team-c")
			reconcileCN(ctx, "us1-team-c")
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "keep", Namespace: "us1-team-c"},
			})).To(Succeed())

			cn := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-c", Namespace: "default"}, cn)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cn)).To(Succeed())
			reconcileCN(ctx, "us1-team-c")

			// object still exists: finalizer held, draining reported, and
			// the namespace itself is NOT deleted while it has contents
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-c", Namespace: "default"}, cn)).To(Succeed())
			Expect(cn.DeletionTimestamp).NotTo(BeNil())
			draining := metaFindCondition(cn.Status.Conditions)
			Expect(draining).NotTo(BeNil())
			Expect(draining.Status).To(Equal(metav1.ConditionFalse))
			Expect(draining.Reason).To(Equal("NamespaceDraining"))
			ns := &corev1.Namespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-c"}, ns)).To(Succeed())
			Expect(ns.DeletionTimestamp).To(BeNil())
		})

		It("completes deletion once the namespace is empty", func() {
			createCN(ctx, "us1-team-d")
			reconcileCN(ctx, "us1-team-d")
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "temp", Namespace: "us1-team-d"},
			})).To(Succeed())

			cn := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-d", Namespace: "default"}, cn)).To(Succeed())
			Expect(k8sClient.Delete(ctx, cn)).To(Succeed())
			reconcileCN(ctx, "us1-team-d")

			// empty the namespace; then it can finish terminating
			Expect(k8sClient.Delete(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "temp", Namespace: "us1-team-d"},
			})).To(Succeed())
			hardDeleteNamespace(ctx, "us1-team-d")

			reconcileCN(ctx, "us1-team-d")
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "us1-team-d", Namespace: "default"}, &customv1.CustomNamespace{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})

// createCN creates a CustomNamespace object in the default namespace.
func createCN(ctx context.Context, name string) {
	cn := &customv1.CustomNamespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       customv1.CustomNamespaceSpec{},
	}
	Expect(k8sClient.Create(ctx, cn)).To(Succeed())
}

// reconcileCN runs one reconcile pass for the named CustomNamespace.
func reconcileCN(ctx context.Context, name string) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	Expect(err).NotTo(HaveOccurred())
	_, err = (&CustomNamespaceReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Discovery: discoveryClient}).Reconcile(ctx,
		reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}})
	Expect(err).NotTo(HaveOccurred())
}

// hardDeleteNamespace removes a namespace the way a real cluster's
// namespace controller finishes it: delete, then clear spec.finalizers
// via the finalize subresource. envtest runs no controllers, so tests
// must simulate this last step themselves.
func hardDeleteNamespace(ctx context.Context, name string) {
	Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})).To(Succeed())
	Eventually(func() bool {
		ns := &corev1.Namespace{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, ns)
		if apierrors.IsNotFound(err) {
			return true
		}
		Expect(err).NotTo(HaveOccurred())
		ns.Spec.Finalizers = nil
		Expect(k8sClient.SubResource("finalize").Update(ctx, ns)).To(Succeed())
		return false
	}).Should(BeTrue())
}

// metaFindCondition returns the Ready condition, or nil.
func metaFindCondition(conditions []metav1.Condition) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == "Ready" {
			return &conditions[i]
		}
	}
	return nil
}
