package controllers

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	batchv1alpha1 "mnmdiagnostics/nextflow-k8s-operator/api/v1alpha1"
)

var _ = Describe("NextflowLaunch controller", func() {

	Context("When creating a NextflowLaunch object", func() {

		It("Should spawn a Pod and a ConfigMap", func() {

			///
			By("Creating a NextflowLaunch object")
			ctx := context.Background()
			nfLaunch := &batchv1alpha1.NextflowLaunch{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-launch",
					Namespace: "default",
				},
				Spec: batchv1alpha1.NextflowLaunchSpec{
					Pipeline: batchv1alpha1.NextflowLaunchPipeline{
						Source: "hello",
					},
					K8s: map[string]string{
						"storageClaimName": "test-pvc",
					},
				},
			}
			Expect(k8sClient.Create(ctx, nfLaunch)).Should(Succeed())

			///
			By("Retrieving the launch from k8s")
			lookupKey := types.NamespacedName{
				Name:      "test-launch",
				Namespace: "default",
			}
			testLaunch := &batchv1alpha1.NextflowLaunch{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, testLaunch)
				if err != nil {
					return false
				}
				return true
			}, 10*time.Second, time.Second).Should(BeTrue())

			///
			By("Checking driver reference")
			var pod *corev1.ObjectReference

			Eventually(func() bool {
				k8sClient.Get(ctx, lookupKey, testLaunch)
				pod = testLaunch.Status.MainPod
				fmt.Fprintf(GinkgoWriter, testLaunch.Status.Stage)
				if pod == nil {
					return false
				}
				return true
			}, 10*time.Second, time.Second).Should(BeTrue())

			podName := testLaunch.Status.MainPod.Name
			fmt.Fprintf(GinkgoWriter, "\n~Driver: "+podName+"~\n")
			Expect(podName).NotTo(BeZero())

			///
			By("Retrieving the pod from k8s")
			lookupKey = types.NamespacedName{
				Name:      testLaunch.Status.MainPod.Name,
				Namespace: testLaunch.Status.MainPod.Namespace,
			}
			testPod := &corev1.Pod{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, testPod)
				if err != nil {
					return false
				}
				return true
			}, 10*time.Second, time.Second).Should(BeTrue())

			driverImage := testPod.Spec.Containers[0].Image
			fmt.Fprintf(GinkgoWriter, "\n~Driver image: "+driverImage+"~\n")
			Expect(driverImage).To(ContainSubstring("nextflow"))

			///
			By("Retrieving the configmap")
			lookupKey = types.NamespacedName{
				Name:      testLaunch.Status.ConfigMap.Name,
				Namespace: testLaunch.Status.ConfigMap.Namespace,
			}
			testConfigMap := &corev1.ConfigMap{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, lookupKey, testConfigMap)
				if err != nil {
					return false
				}
				return true
			}, 10*time.Second, time.Second).Should(BeTrue())
		})
	})
})
