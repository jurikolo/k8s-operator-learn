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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

var _ = Describe("CustomDeployment Controller", func() {
	ctx := context.Background()

	replicas := func(n int32) *int32 { return &n }

	createCD := func(name, ns, deploymentName, image string, repl *int32) *customv1.CustomDeployment {
		cd := &customv1.CustomDeployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       customv1.CustomDeploymentSpec{Namespace: ns, Name: deploymentName, Image: image, Replicas: repl},
		}
		Expect(k8sClient.Create(ctx, cd)).To(Succeed())
		return cd
	}

	reconcileCD := func(name string) {
		_, err := (&CustomDeploymentReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
			reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}})
		Expect(err).NotTo(HaveOccurred())
	}

	getDeployment := func(ns, name string) *appsv1.Deployment {
		d := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, d)).To(Succeed())
		return d
	}

	// podTemplate builds the minimal single-container pod template the
	// operator creates; reused for the unmanaged "rogue" Deployment.
	podTemplate := func(name, image string) corev1.PodTemplateSpec {
		return corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: name, Image: image}}},
		}
	}

	It("fails on a missing CustomNamespace and recovers when it appears", func() {
		createCD("us2-ghost", "us2-missing", "ghost", "nginx:1.27", nil)
		reconcileCD("us2-ghost")

		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-ghost"}, cd)).To(Succeed())
		cond := metaFindCondition(cd.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("MissingCustomNamespace"))
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "us2-missing", Name: "ghost"}, &appsv1.Deployment{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		createCN(ctx, "us2-missing")
		reconcileCN(ctx, "us2-missing")
		reconcileCD("us2-ghost")
		Expect(getDeployment("us2-missing", "ghost").Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-ghost"}, cd)).To(Succeed())
		cond = metaFindCondition(cd.Status.Conditions)
		Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("defaults replicas to 1 and follows updates including scale to zero", func() {
		createCN(ctx, "us2-scale")
		reconcileCN(ctx, "us2-scale")

		createCD("us2-scale-cr", "us2-scale", "web", "nginx:1.27", nil)
		reconcileCD("us2-scale-cr")
		Expect(*getDeployment("us2-scale", "web").Spec.Replicas).To(Equal(int32(1)))

		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-scale-cr"}, cd)).To(Succeed())
		cd.Spec.Replicas = replicas(3)
		Expect(k8sClient.Update(ctx, cd)).To(Succeed())
		reconcileCD("us2-scale-cr")
		Expect(*getDeployment("us2-scale", "web").Spec.Replicas).To(Equal(int32(3)))

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-scale-cr"}, cd)).To(Succeed())
		cd.Spec.Replicas = replicas(0)
		Expect(k8sClient.Update(ctx, cd)).To(Succeed())
		reconcileCD("us2-scale-cr")
		Expect(*getDeployment("us2-scale", "web").Spec.Replicas).To(Equal(int32(0)))
	})

	It("reports a name conflict and leaves an unmanaged Deployment untouched", func() {
		createCN(ctx, "us2-conflict")
		reconcileCN(ctx, "us2-conflict")
		rogue := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "taken", Namespace: "us2-conflict"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "taken"}},
				Template: podTemplate("taken", "rogue:1.0"),
			},
		}
		Expect(k8sClient.Create(ctx, rogue)).To(Succeed())

		createCD("us2-conflict-cr", "us2-conflict", "taken", "nginx:1.27", nil)
		reconcileCD("us2-conflict-cr")

		Expect(getDeployment("us2-conflict", "taken").Spec.Template.Spec.Containers[0].Image).To(Equal("rogue:1.0"))
		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-conflict-cr"}, cd)).To(Succeed())
		cond := metaFindCondition(cd.Status.Conditions)
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("NameConflict"))
	})

	It("deletes the real Deployment when the custom object is deleted", func() {
		createCN(ctx, "us2-cascade")
		reconcileCN(ctx, "us2-cascade")
		createCD("us2-cascade-cr", "us2-cascade", "doomed", "nginx:1.27", nil)
		reconcileCD("us2-cascade-cr")
		getDeployment("us2-cascade", "doomed")

		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-cascade-cr"}, cd)).To(Succeed())
		Expect(k8sClient.Delete(ctx, cd)).To(Succeed())
		reconcileCD("us2-cascade-cr")
		reconcileCD("us2-cascade-cr")

		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "us2-cascade", Name: "doomed"}, &appsv1.Deployment{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-cascade-cr"}, &customv1.CustomDeployment{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("repairs out-of-band image drift", func() {
		createCN(ctx, "us2-drift")
		reconcileCN(ctx, "us2-drift")
		createCD("us2-drift-cr", "us2-drift", "drifty", "nginx:1.27", nil)
		reconcileCD("us2-drift-cr")

		d := getDeployment("us2-drift", "drifty")
		d.Spec.Template.Spec.Containers[0].Image = "hacked:latest"
		Expect(k8sClient.Update(ctx, d)).To(Succeed())

		reconcileCD("us2-drift-cr")
		Expect(getDeployment("us2-drift", "drifty").Spec.Template.Spec.Containers[0].Image).To(Equal("nginx:1.27"))
	})

	It("rejects changing the namespace reference", func() {
		createCN(ctx, "us2-a")
		reconcileCN(ctx, "us2-a")
		createCD("us2-imm", "us2-a", "imm", "nginx:1.27", nil)
		reconcileCD("us2-imm")

		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-imm"}, cd)).To(Succeed())
		cd.Spec.Namespace = "us2-scale"
		err := k8sClient.Update(ctx, cd)
		Expect(err).To(MatchError(ContainSubstring("immutable")))
	})

	It("rejects changing spec.name", func() {
		createCN(ctx, "us2-name-imm")
		reconcileCN(ctx, "us2-name-imm")
		createCD("us2-name-imm-cr", "us2-name-imm", "immname", "nginx:1.27", nil)

		cd := &customv1.CustomDeployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us2-name-imm-cr"}, cd)).To(Succeed())
		cd.Spec.Name = "renamed"
		Expect(k8sClient.Update(ctx, cd)).To(MatchError(ContainSubstring("immutable")))
	})

	It("places real resources in the real namespace named by the CustomNamespace's spec.name", func() {
		// the CustomNamespace object's own name differs from its spec.name
		Expect(k8sClient.Create(ctx, &customv1.CustomNamespace{
			ObjectMeta: metav1.ObjectMeta{Name: "us2-place-ref", Namespace: "default"},
			Spec:       customv1.CustomNamespaceSpec{Name: "us2-place-real"},
		})).To(Succeed())
		reconcileCN(ctx, "us2-place-ref")

		createCD("us2-place-cd", "us2-place-ref", "placed", "nginx:1.27", nil)
		reconcileCD("us2-place-cd")
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "us2-place-real", Name: "placed"}, &appsv1.Deployment{})).To(Succeed())

		Expect(k8sClient.Create(ctx, &customv1.CustomConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "us2-place-cc", Namespace: "default"},
			Spec:       customv1.CustomConfigSpec{Namespace: "us2-place-ref", Name: "us2-place-cc"},
		})).To(Succeed())
		_, err := (&CustomConfigReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
			reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "default", Name: "us2-place-cc"}})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "us2-place-real", Name: "us2-place-cc"}, &corev1.ConfigMap{})).To(Succeed())
	})
})
