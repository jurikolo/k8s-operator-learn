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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	customv1 "github.com/jurikolo/k8s-operator-learn/api/v1"
)

var _ = Describe("CustomConfig Controller", func() {
	ctx := context.Background()

	createCC := func(name, ns string, data map[string]string) *customv1.CustomConfig {
		cc := &customv1.CustomConfig{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
			Spec:       customv1.CustomConfigSpec{Namespace: ns, Data: data},
		}
		Expect(k8sClient.Create(ctx, cc)).To(Succeed())
		return cc
	}

	reconcileCC := func(name string) {
		_, err := (&CustomConfigReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}).Reconcile(ctx,
			reconcile.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}})
		Expect(err).NotTo(HaveOccurred())
	}

	getCM := func(ns, name string) *corev1.ConfigMap {
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, cm)).To(Succeed())
		return cm
	}

	It("fails on a missing CustomNamespace and recovers when it appears", func() {
		createCC("us3-ghost", "us3-missing", map[string]string{"k": "v"})
		reconcileCC("us3-ghost")

		cc := &customv1.CustomConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-ghost"}, cc)).To(Succeed())
		cond := metaFindCondition(cc.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("MissingCustomNamespace"))
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "us3-missing", Name: "us3-ghost"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		createCN(ctx, "us3-missing")
		reconcileCN(ctx, "us3-missing")
		reconcileCC("us3-ghost")
		Expect(getCM("us3-missing", "us3-ghost").Data).To(Equal(map[string]string{"k": "v"}))
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-ghost"}, cc)).To(Succeed())
		Expect(metaFindCondition(cc.Status.Conditions).Status).To(Equal(metav1.ConditionTrue))
	})

	It("carries data and binaryData verbatim and follows content updates", func() {
		createCN(ctx, "us3-content")
		reconcileCN(ctx, "us3-content")

		cc := createCC("us3-content-cr", "us3-content", map[string]string{"log_level": "debug"})
		cc.Spec.BinaryData = map[string][]byte{"blob": {0x01, 0x02}}
		cc.Spec.Immutable = func() *bool { b := false; return &b }()
		Expect(k8sClient.Update(ctx, cc)).To(Succeed())
		reconcileCC("us3-content-cr")

		cm := getCM("us3-content", "us3-content-cr")
		Expect(cm.Data).To(Equal(map[string]string{"log_level": "debug"}))
		Expect(cm.BinaryData).To(Equal(map[string][]byte{"blob": {0x01, 0x02}}))
		Expect(*cm.Immutable).To(BeFalse())

		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-content-cr"}, cc)).To(Succeed())
		cc.Spec.Data = map[string]string{"log_level": "info"}
		Expect(k8sClient.Update(ctx, cc)).To(Succeed())
		reconcileCC("us3-content-cr")
		Expect(getCM("us3-content", "us3-content-cr").Data).To(Equal(map[string]string{"log_level": "info"}))
	})

	It("reports a name conflict and leaves an unmanaged ConfigMap untouched", func() {
		createCN(ctx, "us3-conflict")
		reconcileCN(ctx, "us3-conflict")
		rogue := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "taken", Namespace: "us3-conflict"},
			Data:       map[string]string{"owner": "someone-else"},
		}
		Expect(k8sClient.Create(ctx, rogue)).To(Succeed())

		// the real ConfigMap is named after the custom object, so the CR
		// itself must be named "taken" to collide with the rogue ConfigMap
		createCC("taken", "us3-conflict", map[string]string{"owner": "operator"})
		reconcileCC("taken")

		Expect(getCM("us3-conflict", "taken").Data).To(Equal(map[string]string{"owner": "someone-else"}))
		cc := &customv1.CustomConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "taken"}, cc)).To(Succeed())
		cond := metaFindCondition(cc.Status.Conditions)
		Expect(cond.Status).To(Equal(metav1.ConditionFalse))
		Expect(cond.Reason).To(Equal("NameConflict"))
	})

	It("deletes the real ConfigMap when the custom object is deleted", func() {
		createCN(ctx, "us3-cascade")
		reconcileCN(ctx, "us3-cascade")
		createCC("us3-cascade-cr", "us3-cascade", nil)
		reconcileCC("us3-cascade-cr")
		getCM("us3-cascade", "us3-cascade-cr")

		cc := &customv1.CustomConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-cascade-cr"}, cc)).To(Succeed())
		Expect(k8sClient.Delete(ctx, cc)).To(Succeed())
		reconcileCC("us3-cascade-cr")
		reconcileCC("us3-cascade-cr")

		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: "us3-cascade", Name: "us3-cascade-cr"}, &corev1.ConfigMap{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-cascade-cr"}, &customv1.CustomConfig{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("rejects changing the namespace reference", func() {
		createCN(ctx, "us3-a")
		reconcileCN(ctx, "us3-a")
		createCC("us3-imm", "us3-a", nil)
		reconcileCC("us3-imm")

		cc := &customv1.CustomConfig{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: "default", Name: "us3-imm"}, cc)).To(Succeed())
		cc.Spec.Namespace = "us3-content"
		err := k8sClient.Update(ctx, cc)
		Expect(err).To(MatchError(ContainSubstring("immutable")))
	})
})
