/*
Copyright 2024.

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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/4ndersson/epg-config-operator/api/v1alpha1"
	"github.com/4ndersson/epg-config-operator/pkg/aci"
)

var _ = Describe("Epgconf Controller", func() {
	const (
		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	ctx := context.Background()

	conf := &v1alpha1.Epgconf{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "epg.custom.aci/v1alpha1",
			Kind:       "Conf",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "epg-sync-test",
			Namespace: "ns-1",
		},
	}

	Context("When creating a new Epgconf resource", func() {
		It("Should create the Epgconf resource", func() {
			By("Creating namespace", func() {
				namespace := &corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Namespace",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name: conf.Namespace,
					},
				}
				Expect(k8sClient.Create(ctx, namespace)).Should(Succeed())
			})
			By("Creating the EpgConf resource", func() {
				Expect(k8sClient.Create(ctx, conf)).Should(Succeed())

				lookupKey := types.NamespacedName{Name: conf.Name, Namespace: conf.ObjectMeta.Namespace}
				created := &v1alpha1.Epgconf{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, lookupKey, created)
					return err == nil
				}, timeout, interval).Should(BeTrue())
				Expect(created.Name).Should(Equal(conf.Name))

				reconciler := &EpgconfReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					ApicClient: apicClient,
					CniConfig:  cniConf,
				}
				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: lookupKey})
				Expect(err).ShouldNot(HaveOccurred())
			})
			By("Checking the annotation", func() {
				namespace := &corev1.Namespace{}
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: conf.Namespace}, namespace)
					if err != nil {
						return false
					}
					annotation := namespace.Annotations["opflex.cisco.com/endpoint-group"]
					expectedAnnotation := fmt.Sprintf(`{"tenant":"%s","app-profile":"%s","name":"%s_EPG"}`, cniConf.Tenant, cniConf.ApplicationProfile, conf.Namespace)
					return annotation == expectedAnnotation
				}, timeout, interval).Should(BeTrue(), "Namespace should have the correct annotation")
			})
			By("Checking if EPG exists", func() {
				Eventually(func() bool {
					exists, _ := apicClient.EpgExists(conf.Namespace+"_EPG", cniConf.ApplicationProfile, cniConf.Tenant)
					return exists
				}, timeout, interval).Should(BeTrue())
			})
			By("Checking EPG Configuration", func() {
				epg := apicClient.(*aci.ApicClientMocks).GetEpg(conf.ObjectMeta.Namespace+"_EPG", cniConf.ApplicationProfile, cniConf.Tenant)
				Expect(epg.Vmm).Should(Equal(cniConf.VmmDomain))
				Expect(epg.VmmType).Should(Equal(cniConf.VmmDomainType))
				Expect(epg.Bd).Should(Equal(cniConf.BridgeDomain))
			})
			By("Checking consumed contracts", func() {
				contracts, _ := apicClient.GetConsumedContracts(conf.ObjectMeta.Namespace+"_EPG", cniConf.ApplicationProfile, cniConf.Tenant)
				Expect(contracts).Should(Equal(cniConf.ConsumedContracts))
			})
			By("Checking provided contracts", func() {
				contracts, _ := apicClient.GetProvidedContracts(conf.ObjectMeta.Namespace+"_EPG", cniConf.ApplicationProfile, cniConf.Tenant)
				Expect(contracts).Should(Equal(cniConf.ProvidedContracts))
			})
		})
	})
	Context("When deleting the EpgConf resource", func() {
		It("It should delete the EpgConf resource and clean up associated resources", func() {
			By("Deleting the EpgConf resource", func() {
				Expect(k8sClient.Delete(ctx, conf)).Should(Succeed())

				reconciler := &EpgconfReconciler{
					Client:     k8sClient,
					Scheme:     k8sClient.Scheme(),
					ApicClient: apicClient,
					CniConfig:  cniConf,
				}
				_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: conf.Name, Namespace: conf.Namespace}})
				Expect(err).ShouldNot(HaveOccurred())

				lookupKey := types.NamespacedName{Name: conf.Name, Namespace: conf.Namespace}
				deletedconf := &v1alpha1.Epgconf{}
				Eventually(func() string {
					err := k8sClient.Get(ctx, lookupKey, deletedconf)
					if err != nil {
						if errors.IsNotFound(err) {
							return "" // Resource is fully deleted
						}
						return fmt.Sprintf("Error getting conf: %v", err)
					}
					if len(deletedconf.Finalizers) > 0 {
						return fmt.Sprintf("conf resource still has finalizers: %v", deletedconf.Finalizers)
					}
					return fmt.Sprintf("conf resource still exists: %+v", deletedconf)
				}, timeout, interval).Should(BeEmpty(), "conf resource should be deleted")
			})
			By("Checking if EPG is removed", func() {
				Eventually(func() bool {
					exists, _ := apicClient.EpgExists(conf.Namespace+"_EPG", cniConf.ApplicationProfile, cniConf.Tenant)
					return !exists
				}, timeout, interval).Should(BeTrue())
			})
		})
	})
})
