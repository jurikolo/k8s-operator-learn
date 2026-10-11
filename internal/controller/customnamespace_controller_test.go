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
					Spec:       customv1.CustomNamespaceSpec{Name: name},
				})
				Expect(err).To(MatchError(ContainSubstring("reserved")), "name: %s", name)
			}
		})

		It("rejects a missing spec.name", func() {
			// an empty Go string marshals as name:"" and trips the pattern
			// rule; a manifest omitting spec.name trips "Required value" —
			// both rejections name spec.name
			err := k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-no-name", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{},
			})
			Expect(err).To(MatchError(ContainSubstring("spec.name")))
		})

		It("rejects pattern-invalid names", func() {
			err := k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-bad-pattern", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "Team-Apps"},
			})
			Expect(err).To(MatchError(ContainSubstring("spec.name")))
		})

		It("rejects changing spec.name", func() {
			createCN(ctx, "us1-immutable")
			cn := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-immutable", Namespace: "default"}, cn)).To(Succeed())
			cn.Spec.Name = "renamed"
			Expect(k8sClient.Update(ctx, cn)).To(MatchError(ContainSubstring("immutable")))
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

		It("resolves duplicate claims first-claim-wins", func() {
			createCN(ctx, "us1-dupe-first")
			reconcileCN(ctx, "us1-dupe-first")

			// a second object, differently named, claiming the same spec.name
			Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-dupe-second", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "us1-dupe-first"},
			})).To(Succeed())
			reconcileCN(ctx, "us1-dupe-second")

			first := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-dupe-first", Namespace: "default"}, first)).To(Succeed())
			Expect(metaFindCondition(first.Status.Conditions).Reason).To(Equal("Ready"))

			second := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-dupe-second", Namespace: "default"}, second)).To(Succeed())
			cond := metaFindCondition(second.Status.Conditions)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NameConflict"))
			Expect(cond.Message).To(ContainSubstring("CustomNamespace.default.us1-dupe-first"))
		})

		It("reports a name conflict and leaves a pre-existing unmanaged namespace untouched", func() {
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-pre"},
			})).To(Succeed())
			Expect(k8sClient.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "theirs", Namespace: "us1-pre"},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-pre-cr", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "us1-pre"},
			})).To(Succeed())
			reconcileCN(ctx, "us1-pre-cr")

			cn := &customv1.CustomNamespace{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-pre-cr", Namespace: "default"}, cn)).To(Succeed())
			cond := metaFindCondition(cn.Status.Conditions)
			Expect(cond.Status).To(Equal(metav1.ConditionFalse))
			Expect(cond.Reason).To(Equal("NameConflict"))
			// namespace and its contents are untouched
			Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: "us1-pre", Name: "theirs"}, &corev1.ConfigMap{})).To(Succeed())
		})

		It("does not delete a foreign namespace when a losing claimant is deleted", func() {
			createCN(ctx, "us1-win")
			reconcileCN(ctx, "us1-win")

			Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-lose", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "us1-win"},
			})).To(Succeed())
			reconcileCN(ctx, "us1-lose")
			Expect(k8sClient.Delete(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "us1-lose", Namespace: "default"},
			})).To(Succeed())
			reconcileCN(ctx, "us1-lose")

			// the loser releases its finalizer without touching the winner's namespace
			err := k8sClient.Get(ctx, client.ObjectKey{Name: "us1-lose", Namespace: "default"}, &customv1.CustomNamespace{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "us1-win"}, &corev1.Namespace{})).To(Succeed())
		})

		It("creates real namespaces under spec.name, ignoring the object names", func() {
			Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "some-object-name", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "team-apps"},
			})).To(Succeed())
			Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
				ObjectMeta: metav1.ObjectMeta{Name: "another-object-name", Namespace: "default"},
				Spec:       customv1.CustomNamespaceSpec{Name: "team-b"},
			})).To(Succeed())
			reconcileCN(ctx, "some-object-name")
			reconcileCN(ctx, "another-object-name")

			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "team-apps"}, &corev1.Namespace{})).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: "team-b"}, &corev1.Namespace{})).To(Succeed())
		})
	})
})

// createCN creates a CustomNamespace object in the default namespace; the
// real namespace follows spec.name, here set to the object's own name.
func createCN(ctx context.Context, name string) {
	cn := &customv1.CustomNamespace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       customv1.CustomNamespaceSpec{Name: name},
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
