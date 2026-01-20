/*
Copyright 2025.

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

	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// fetchAndStoreKubeconfig fetches the kubeconfig from AKS and stores it in the state secret
// Returns true if kubeconfig was updated, false if it already existed
func (r *KubernetesClusterReconciler) fetchAndStoreKubeconfig(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (bool, error) {
	// Check if kubeconfig already exists
	existingKubeconfig, err := r.ClusterStateService.GetKubeconfig(ctx, kubernetesCluster)
	if err == nil && len(existingKubeconfig) > 0 {
		// Kubeconfig already stored, no need to fetch again
		return false, nil
	}

	credentials, err := r.AKSClusterManager.GetClusterCredentials(ctx, kubernetesCluster)
	if err != nil {
		return false, fmt.Errorf("failed to get cluster credentials: %w", err)
	}

	if credentials != nil && credentials.Kubeconfigs != nil && len(credentials.Kubeconfigs) > 0 {
		kubeconfig := credentials.Kubeconfigs[0].Value
		if kubeconfig != nil {
			if err := r.ClusterStateService.SetKubeconfig(ctx, kubernetesCluster, kubeconfig); err != nil {
				return false, fmt.Errorf("failed to store kubeconfig: %w", err)
			}
			vlog.Info("Stored kubeconfig in cluster state secret", "cluster", kubernetesCluster.Name)
			return true, nil
		}
	}

	return false, nil
}

// fetchAndStoreKubeSystemUID fetches the kube-system namespace UID from the guest cluster
// and stores it in the state secret and as an annotation on the KubernetesCluster CR.
// Returns true if the UID was updated, false if it already existed.
func (r *KubernetesClusterReconciler) fetchAndStoreKubeSystemUID(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (bool, error) {
	// Check if UID already exists in annotation
	if kubernetesCluster.Annotations != nil {
		if _, ok := kubernetesCluster.Annotations[AnnotationKubeSystemUID]; ok {
			// Also verify it's in the secret
			existingUID, err := r.ClusterStateService.GetKubeSystemUID(ctx, kubernetesCluster)
			if err == nil && existingUID != "" {
				return false, nil // Already stored
			}
		}
	}

	// Check if UID exists in secret but not in annotation
	existingUID, _ := r.ClusterStateService.GetKubeSystemUID(ctx, kubernetesCluster)
	if existingUID != "" {
		// Secret has it, but annotation might be missing - update annotation
		if kubernetesCluster.Annotations == nil {
			kubernetesCluster.Annotations = make(map[string]string)
		}
		if kubernetesCluster.Annotations[AnnotationKubeSystemUID] != existingUID {
			kubernetesCluster.Annotations[AnnotationKubeSystemUID] = existingUID
			if err := r.Update(ctx, kubernetesCluster); err != nil {
				return false, fmt.Errorf("failed to update annotation: %w", err)
			}
		}
		return false, nil
	}

	// Get kubeconfig from state secret
	kubeconfig, err := r.ClusterStateService.GetKubeconfig(ctx, kubernetesCluster)
	if err != nil || len(kubeconfig) == 0 {
		return false, fmt.Errorf("kubeconfig not available yet")
	}

	// Create client config from kubeconfig
	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfig)
	if err != nil {
		return false, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return false, fmt.Errorf("failed to create rest config: %w", err)
	}

	// Create kubernetes client for the guest cluster
	guestClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return false, fmt.Errorf("failed to create guest cluster client: %w", err)
	}

	// Fetch kube-system namespace
	kubeSystemNS, err := guestClient.CoreV1().Namespaces().Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get kube-system namespace: %w", err)
	}

	kubeSystemUID := string(kubeSystemNS.UID)

	// Store in state secret
	if err := r.ClusterStateService.SetKubeSystemUID(ctx, kubernetesCluster, kubeSystemUID); err != nil {
		return false, fmt.Errorf("failed to store kube-system UID in secret: %w", err)
	}

	// Store in annotation
	if kubernetesCluster.Annotations == nil {
		kubernetesCluster.Annotations = make(map[string]string)
	}
	kubernetesCluster.Annotations[AnnotationKubeSystemUID] = kubeSystemUID
	if err := r.Update(ctx, kubernetesCluster); err != nil {
		return false, fmt.Errorf("failed to update annotation: %w", err)
	}

	vlog.Info("🆔 Stored kube-system UID", "cluster", kubernetesCluster.Name, "uid", kubeSystemUID)
	return true, nil
}
