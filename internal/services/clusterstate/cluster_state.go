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

package clusterstate

import (
	"context"
	"time"

	"github.com/vitistack/common/pkg/loggers/vlog"
	"github.com/vitistack/common/pkg/services/secretservice"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// State keys for the cluster state secret
const (
	// Timestamps
	KeyCreatedAt = "created_at"
	KeyReadyAt   = "ready_at"

	// Provisioning states
	KeyClusterProvisioning = "cluster_provisioning"
	KeyClusterProvisioned  = "cluster_provisioned"
	KeyNodePoolsReady      = "nodepools_ready"

	// Access states
	KeyKubernetesAPIReady = "kubernetes_api_ready"
	KeyClusterAccess      = "cluster_access"
	KeyKubeconfigPresent  = "kubeconfig_present"

	// Kubeconfig data
	KeyKubeConfig = "kube.config"

	// Azure specific
	KeyAKSResourceID       = "aks_resource_id"
	KeyAKSProvisionState   = "aks_provision_state"
	KeyResourceGroupName   = "resource_group_name"
	KeyAKSClusterName      = "aks_cluster_name"
	KeyAKSClusterFQDN      = "aks_cluster_fqdn"
	KeyNodePoolsConfigured = "nodepools_configured"

	// Label for the secret
	LabelClusterID = "vitistack.io/clusterid"
)

// ClusterStateService manages the state secret for a cluster
type ClusterStateService struct {
	secretService *secretservice.SecretService
	client        client.Client
}

// NewClusterStateService creates a new ClusterStateService
func NewClusterStateService(c client.Client) *ClusterStateService {
	return &ClusterStateService{
		secretService: secretservice.NewSecretService(c),
		client:        c,
	}
}

// GetOrCreateState gets or creates the cluster state secret
func (s *ClusterStateService) GetOrCreateState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (*corev1.Secret, error) {
	secretName := cluster.Spec.Cluster.ClusterId
	namespace := cluster.Namespace

	secret, err := s.secretService.GetSecret(ctx, secretName, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new state secret
			return s.createStateSecret(ctx, cluster)
		}
		return nil, err
	}

	return secret, nil
}

// getStateWithRetry gets the cluster state secret with retry for cache consistency
// This is useful after create/update operations where the cache might be stale
func (s *ClusterStateService) getStateWithRetry(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (*corev1.Secret, error) {
	secretName := cluster.Spec.Cluster.ClusterId
	namespace := cluster.Namespace

	var secret *corev1.Secret
	var err error

	// Try up to 3 times with a small delay to handle cache inconsistency
	for range 3 {
		secret, err = s.secretService.GetSecret(ctx, secretName, namespace)
		if err == nil {
			return secret, nil
		}
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		// Small delay before retry for cache to sync
		time.Sleep(100 * time.Millisecond)
	}

	return nil, err
}

// createStateSecret creates a new state secret for the cluster
func (s *ClusterStateService) createStateSecret(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (*corev1.Secret, error) {
	secretName := cluster.Spec.Cluster.ClusterId
	namespace := cluster.Namespace

	labels := map[string]string{
		LabelClusterID: cluster.Spec.Cluster.ClusterId,
	}

	data := map[string][]byte{
		KeyCreatedAt:           []byte(time.Now().UTC().Format(time.RFC3339)),
		KeyClusterProvisioning: []byte("false"),
		KeyClusterProvisioned:  []byte("false"),
		KeyNodePoolsReady:      []byte("false"),
		KeyKubernetesAPIReady:  []byte("false"),
		KeyClusterAccess:       []byte("false"),
		KeyKubeconfigPresent:   []byte("false"),
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Set owner reference so the secret is deleted when the cluster is deleted
	if err := controllerutil.SetControllerReference(cluster, secret, s.client.Scheme()); err != nil {
		vlog.Error(err, "failed to set owner reference on state secret", "cluster", cluster.Name)
		// Continue without owner reference - secret will be orphaned but functional
	}

	if err := s.client.Create(ctx, secret); err != nil {
		// If secret already exists (race condition), fetch and return it
		if apierrors.IsAlreadyExists(err) {
			vlog.Info("State secret already exists, fetching it", "cluster", cluster.Name, "secret", secretName)
			return s.getStateWithRetry(ctx, cluster)
		}
		return nil, err
	}

	vlog.Info("Created cluster state secret", "cluster", cluster.Name, "secret", secretName)

	// Re-fetch the secret to get the correct ResourceVersion for subsequent updates
	return s.getStateWithRetry(ctx, cluster)
}

// UpdateState updates a specific key in the state secret
func (s *ClusterStateService) UpdateState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, key, value string) error {
	// Retry up to 3 times to handle conflicts from stale ResourceVersion
	var lastErr error
	for range 3 {
		secret, err := s.GetOrCreateState(ctx, cluster)
		if err != nil {
			return err
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[key] = []byte(value)

		err = s.secretService.UpdateSecret(ctx, secret)
		if err == nil {
			return nil
		}
		lastErr = err

		// If conflict, retry with fresh secret
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
	return lastErr
}

// UpdateStateMultiple updates multiple keys in the state secret
func (s *ClusterStateService) UpdateStateMultiple(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, updates map[string]string) error {
	// Retry up to 3 times to handle conflicts from stale ResourceVersion
	var lastErr error
	for range 3 {
		secret, err := s.GetOrCreateState(ctx, cluster)
		if err != nil {
			return err
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		for key, value := range updates {
			secret.Data[key] = []byte(value)
		}

		err = s.secretService.UpdateSecret(ctx, secret)
		if err == nil {
			return nil
		}
		lastErr = err

		// If conflict, retry with fresh secret
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
	return lastErr
}

// GetState retrieves a specific key from the state secret
func (s *ClusterStateService) GetState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, key string) (string, error) {
	secret, err := s.GetOrCreateState(ctx, cluster)
	if err != nil {
		return "", err
	}

	if value, ok := secret.Data[key]; ok {
		return string(value), nil
	}

	return "", nil
}

// GetAllState retrieves all state data from the secret
func (s *ClusterStateService) GetAllState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (map[string]string, error) {
	secret, err := s.GetOrCreateState(ctx, cluster)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for key, value := range secret.Data {
		result[key] = string(value)
	}

	return result, nil
}

// IsClusterProvisioned checks if the cluster has been provisioned
func (s *ClusterStateService) IsClusterProvisioned(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (bool, error) {
	value, err := s.GetState(ctx, cluster, KeyClusterProvisioned)
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

// IsKubernetesAPIReady checks if the Kubernetes API is ready
func (s *ClusterStateService) IsKubernetesAPIReady(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) (bool, error) {
	value, err := s.GetState(ctx, cluster, KeyKubernetesAPIReady)
	if err != nil {
		return false, err
	}
	return value == "true", nil
}

// SetClusterProvisioning marks the cluster as provisioning
func (s *ClusterStateService) SetClusterProvisioning(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) error {
	return s.UpdateState(ctx, cluster, KeyClusterProvisioning, "true")
}

// SetClusterFailed marks the cluster provisioning as failed
func (s *ClusterStateService) SetClusterFailed(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, reason string) error {
	updates := map[string]string{
		KeyClusterProvisioning: "false",
		KeyClusterProvisioned:  "false",
		KeyAKSProvisionState:   "Failed",
	}
	if reason != "" {
		updates["failure_reason"] = reason
	}
	return s.UpdateStateMultiple(ctx, cluster, updates)
}

// SetClusterProvisioned marks the cluster as provisioned
func (s *ClusterStateService) SetClusterProvisioned(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, aksResourceID, aksClusterName, resourceGroupName string) error {
	updates := map[string]string{
		KeyClusterProvisioning: "false",
		KeyClusterProvisioned:  "true",
		KeyAKSResourceID:       aksResourceID,
		KeyAKSClusterName:      aksClusterName,
		KeyResourceGroupName:   resourceGroupName,
	}
	return s.UpdateStateMultiple(ctx, cluster, updates)
}

// SetKubernetesAPIReady marks the Kubernetes API as ready
func (s *ClusterStateService) SetKubernetesAPIReady(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) error {
	return s.UpdateState(ctx, cluster, KeyKubernetesAPIReady, "true")
}

// SetClusterReady marks the cluster as fully ready
func (s *ClusterStateService) SetClusterReady(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) error {
	updates := map[string]string{
		KeyClusterAccess:      "true",
		KeyKubernetesAPIReady: "true",
		KeyNodePoolsReady:     "true",
		KeyReadyAt:            time.Now().UTC().Format(time.RFC3339),
	}
	return s.UpdateStateMultiple(ctx, cluster, updates)
}

// SetKubeconfig stores the kubeconfig in the state secret
func (s *ClusterStateService) SetKubeconfig(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, kubeconfig []byte) error {
	// Retry up to 3 times to handle conflicts from stale ResourceVersion
	var lastErr error
	for range 3 {
		secret, err := s.GetOrCreateState(ctx, cluster)
		if err != nil {
			return err
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[KeyKubeConfig] = kubeconfig
		secret.Data[KeyKubeconfigPresent] = []byte("true")

		err = s.secretService.UpdateSecret(ctx, secret)
		if err == nil {
			return nil
		}
		lastErr = err

		// If conflict, retry with fresh secret
		if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
	return lastErr
}

// GetKubeconfig retrieves the kubeconfig from the state secret
func (s *ClusterStateService) GetKubeconfig(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) ([]byte, error) {
	secret, err := s.GetOrCreateState(ctx, cluster)
	if err != nil {
		return nil, err
	}

	if kubeconfig, ok := secret.Data[KeyKubeConfig]; ok {
		return kubeconfig, nil
	}

	return nil, nil
}

// SetAKSProvisionState updates the AKS provisioning state
func (s *ClusterStateService) SetAKSProvisionState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, state string) error {
	return s.UpdateState(ctx, cluster, KeyAKSProvisionState, state)
}

// SetClusterFQDN stores the cluster FQDN
func (s *ClusterStateService) SetClusterFQDN(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, fqdn string) error {
	return s.UpdateState(ctx, cluster, KeyAKSClusterFQDN, fqdn)
}

// SetNodePoolsConfigured stores the list of configured node pools
func (s *ClusterStateService) SetNodePoolsConfigured(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster, nodePools string) error {
	return s.UpdateState(ctx, cluster, KeyNodePoolsConfigured, nodePools)
}

// DeleteState deletes the cluster state secret
func (s *ClusterStateService) DeleteState(ctx context.Context, cluster *vitistackv1alpha1.KubernetesCluster) error {
	secretName := cluster.Spec.Cluster.ClusterId
	namespace := cluster.Namespace

	secret, err := s.secretService.GetSecret(ctx, secretName, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil // Already deleted
		}
		return err
	}

	if err := s.client.Delete(ctx, secret); err != nil {
		return err
	}

	vlog.Info("Deleted cluster state secret", "cluster", cluster.Name, "secret", secretName)
	return nil
}
