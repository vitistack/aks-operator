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
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// initializeStatusState ensures all required status.state fields have default values
func (r *KubernetesClusterReconciler) initializeStatusState(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) {
	now := metav1.Now()

	// Initialize timestamps
	if kubernetesCluster.Status.State.Created.IsZero() {
		kubernetesCluster.Status.State.Created = now
	}
	kubernetesCluster.Status.State.LastUpdated = now
	kubernetesCluster.Status.State.LastUpdatedBy = "aks-operator"

	// Initialize endpoints if nil
	if kubernetesCluster.Status.State.Endpoints == nil {
		kubernetesCluster.Status.State.Endpoints = []vitistackv1alpha1.KubernetesClusterEndpoint{}
	}

	// Initialize versions if nil
	if kubernetesCluster.Status.State.Versions == nil {
		kubernetesCluster.Status.State.Versions = []vitistackv1alpha1.KubernetesClusterVersion{
			{Name: "kubernetes", Version: kubernetesCluster.Spec.Topology.Version, Branch: "stable"},
		}
	}

	// Initialize egress IP if empty
	if kubernetesCluster.Status.State.EgressIP == "" {
		kubernetesCluster.Status.State.EgressIP = "pending"
	}

	// Default resource with zero values
	zeroQuantity := resource.MustParse("0")
	defaultResource := vitistackv1alpha1.KubernetesClusterStatusClusterStatusResource{
		Capacity:  zeroQuantity,
		Used:      zeroQuantity,
		Percetage: 0,
	}

	// Initialize cluster resources
	if kubernetesCluster.Status.State.Cluster.Resources.CPU.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.Resources.CPU = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.Resources.Memory.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.Resources.Memory = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.Resources.Disk.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.Resources.Disk = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.Resources.GPU.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.Resources.GPU = defaultResource
	}

	// Initialize controlplane status
	if kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.CPU.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.CPU = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.Memory.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.Memory = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.Disk.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.Disk = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.GPU.Capacity.IsZero() {
		kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Resources.GPU = defaultResource
	}
	if kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Nodes == nil {
		kubernetesCluster.Status.State.Cluster.ControlPlaneStatus.Nodes = []string{}
	}

	// Initialize nodepools
	if kubernetesCluster.Status.State.Cluster.NodePools == nil {
		kubernetesCluster.Status.State.Cluster.NodePools = []vitistackv1alpha1.KubernetesClusterNodePoolStatus{}
	}
}

// updatePhaseAndCondition updates the phase and a specific condition
func (r *KubernetesClusterReconciler) updatePhaseAndCondition(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, phase, conditionType, status, message string) error {
	// Initialize all required status fields with defaults
	r.initializeStatusState(kubernetesCluster)

	kubernetesCluster.Status.Phase = phase

	// Update or add the specific condition
	r.setCondition(kubernetesCluster, conditionType, status, message)

	// Sort conditions by timestamp
	r.sortConditions(kubernetesCluster)

	return r.Status().Update(ctx, kubernetesCluster)
}

// setCondition updates or adds a condition. Only updates timestamp if status or message changed.
// Returns true if the condition was actually changed.
func (r *KubernetesClusterReconciler) setCondition(kubernetesCluster *vitistackv1alpha1.KubernetesCluster, conditionType, status, message string) bool {
	now := time.Now().Format(time.RFC3339)

	// Find existing condition
	for i := range kubernetesCluster.Status.Conditions {
		if kubernetesCluster.Status.Conditions[i].Type == conditionType {
			// Only update if status or message actually changed
			if kubernetesCluster.Status.Conditions[i].Status == status &&
				kubernetesCluster.Status.Conditions[i].Message == message {
				return false // No change
			}
			kubernetesCluster.Status.Conditions[i].Status = status
			kubernetesCluster.Status.Conditions[i].Message = message
			kubernetesCluster.Status.Conditions[i].LastTransitionTime = now
			return true
		}
	}

	// Not found - add new condition
	kubernetesCluster.Status.Conditions = append(kubernetesCluster.Status.Conditions, vitistackv1alpha1.KubernetesClusterCondition{
		Type:               conditionType,
		Status:             status,
		Message:            message,
		LastTransitionTime: now,
	})
	return true
}

// sortConditions sorts conditions by timestamp (most recent first)
func (r *KubernetesClusterReconciler) sortConditions(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) {
	sort.Slice(kubernetesCluster.Status.Conditions, func(i, j int) bool {
		// Sort by LastTransitionTime descending (most recent first)
		return kubernetesCluster.Status.Conditions[i].LastTransitionTime > kubernetesCluster.Status.Conditions[j].LastTransitionTime
	})
}

// updateClusterStatus updates the full cluster status from AKS cluster data
func (r *KubernetesClusterReconciler) updateClusterStatus(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, aksCluster *armcontainerservice.ManagedCluster, phase string) error {
	// Initialize all required status fields with defaults
	r.initializeStatusState(kubernetesCluster)

	// Store the AKS cluster ID in annotations (separate update for metadata)
	if aksCluster.ID != nil {
		if kubernetesCluster.Annotations == nil {
			kubernetesCluster.Annotations = make(map[string]string)
		}
		// Only update annotation if it's different
		if kubernetesCluster.Annotations[AnnotationAKSClusterID] != *aksCluster.ID {
			kubernetesCluster.Annotations[AnnotationAKSClusterID] = *aksCluster.ID
			if err := r.Update(ctx, kubernetesCluster); err != nil {
				vlog.Error(err, "failed to update annotations", "cluster", kubernetesCluster.Name)
				// Don't return error - continue with status update
			}
		}

		// Update external ID in status
		kubernetesCluster.Status.State.Cluster.ExternalId = *aksCluster.ID
	}

	// Update endpoints if FQDN is available
	if aksCluster.Properties != nil && aksCluster.Properties.Fqdn != nil {
		fqdn := *aksCluster.Properties.Fqdn
		kubernetesCluster.Status.State.Endpoints = []vitistackv1alpha1.KubernetesClusterEndpoint{
			{Name: "kubernetes", Address: fmt.Sprintf("https://%s:443", fqdn)},
		}
	}

	// Update phase
	kubernetesCluster.Status.Phase = phase

	// Build status message from provisioning state
	message := "Cluster is " + phase
	provisioningState := "Unknown"
	if aksCluster.Properties != nil && aksCluster.Properties.ProvisioningState != nil {
		provisioningState = *aksCluster.Properties.ProvisioningState
		message = fmt.Sprintf("Cluster is %s (Azure state: %s)", phase, provisioningState)
	}

	// Set conditions based on the current state
	r.updateConditionsFromState(kubernetesCluster, phase, provisioningState, message)

	// Sort conditions
	r.sortConditions(kubernetesCluster)

	// Use Status().Update() with retry on conflict
	err := r.Status().Update(ctx, kubernetesCluster)
	if err != nil {
		// If we get a conflict, re-fetch and try again
		if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "modified") {
			// Re-fetch the latest version
			if fetchErr := r.Get(ctx, client.ObjectKeyFromObject(kubernetesCluster), kubernetesCluster); fetchErr != nil {
				return fmt.Errorf("failed to re-fetch cluster: %w", fetchErr)
			}
			// Re-apply status changes
			r.initializeStatusState(kubernetesCluster)
			if aksCluster.ID != nil {
				kubernetesCluster.Status.State.Cluster.ExternalId = *aksCluster.ID
			}
			if aksCluster.Properties != nil && aksCluster.Properties.Fqdn != nil {
				kubernetesCluster.Status.State.Endpoints = []vitistackv1alpha1.KubernetesClusterEndpoint{
					{Name: "kubernetes", Address: fmt.Sprintf("https://%s:443", *aksCluster.Properties.Fqdn)},
				}
			}
			kubernetesCluster.Status.Phase = phase
			r.updateConditionsFromState(kubernetesCluster, phase, provisioningState, message)
			r.sortConditions(kubernetesCluster)
			return r.Status().Update(ctx, kubernetesCluster)
		}
		return err
	}
	return nil
}

// updateConditionsFromState updates all conditions based on the cluster state
func (r *KubernetesClusterReconciler) updateConditionsFromState(kubernetesCluster *vitistackv1alpha1.KubernetesCluster, phase, provisioningState, message string) {
	// Determine condition statuses based on phase
	var provisionedStatus, readyStatus, apiReadyStatus, kubeconfigStatus, nodepoolsStatus string

	switch phase {
	case PhaseReady:
		provisionedStatus = ConditionStatusOK
		readyStatus = ConditionStatusOK
		apiReadyStatus = ConditionStatusOK
		kubeconfigStatus = ConditionStatusOK
		nodepoolsStatus = ConditionStatusOK
	case PhaseFailed:
		provisionedStatus = ConditionStatusError
		readyStatus = ConditionStatusError
		apiReadyStatus = ConditionStatusError
		kubeconfigStatus = ConditionStatusError
		nodepoolsStatus = ConditionStatusError
	case PhaseCreating, PhaseProvisioning:
		provisionedStatus = ConditionStatusWorking
		readyStatus = ConditionStatusWorking
		apiReadyStatus = ConditionStatusUnknown
		kubeconfigStatus = ConditionStatusUnknown
		nodepoolsStatus = ConditionStatusUnknown
	case PhaseUpdating:
		provisionedStatus = ConditionStatusOK
		readyStatus = ConditionStatusWorking
		apiReadyStatus = ConditionStatusOK
		kubeconfigStatus = ConditionStatusOK
		nodepoolsStatus = ConditionStatusWorking
	case PhaseDeleting:
		provisionedStatus = ConditionStatusWorking
		readyStatus = ConditionStatusWorking
		apiReadyStatus = ConditionStatusUnknown
		kubeconfigStatus = ConditionStatusUnknown
		nodepoolsStatus = ConditionStatusUnknown
	default:
		provisionedStatus = ConditionStatusUnknown
		readyStatus = ConditionStatusUnknown
		apiReadyStatus = ConditionStatusUnknown
		kubeconfigStatus = ConditionStatusUnknown
		nodepoolsStatus = ConditionStatusUnknown
	}

	// Set all conditions and track changes
	changed := false
	changed = r.setCondition(kubernetesCluster, ConditionClusterProvisioned, provisionedStatus, fmt.Sprintf("Azure state: %s", provisioningState)) || changed
	changed = r.setCondition(kubernetesCluster, ConditionClusterReady, readyStatus, message) || changed
	changed = r.setCondition(kubernetesCluster, ConditionKubernetesAPIReady, apiReadyStatus, getAPIReadyMessage(apiReadyStatus)) || changed
	changed = r.setCondition(kubernetesCluster, ConditionKubeConfigReady, kubeconfigStatus, getKubeconfigMessage(kubeconfigStatus)) || changed
	changed = r.setCondition(kubernetesCluster, ConditionNodePoolsReady, nodepoolsStatus, getNodePoolsMessage(nodepoolsStatus)) || changed
	_ = changed // Suppress unused variable - conditions are tracked for status updates
}

// getAPIReadyMessage returns a human-readable message for the API ready condition
func getAPIReadyMessage(status string) string {
	switch status {
	case ConditionStatusOK:
		return "Kubernetes API server is accessible"
	case ConditionStatusError:
		return "Kubernetes API server is not accessible"
	case ConditionStatusWorking:
		return "Waiting for Kubernetes API server"
	default:
		return "Kubernetes API status unknown"
	}
}

// getKubeconfigMessage returns a human-readable message for the kubeconfig condition
func getKubeconfigMessage(status string) string {
	switch status {
	case ConditionStatusOK:
		return "Kubeconfig is available in cluster state secret"
	case ConditionStatusError:
		return "Failed to retrieve kubeconfig"
	case ConditionStatusWorking:
		return "Waiting for kubeconfig"
	default:
		return "Kubeconfig status unknown"
	}
}

// getNodePoolsMessage returns a human-readable message for the node pools condition
func getNodePoolsMessage(status string) string {
	switch status {
	case ConditionStatusOK:
		return "All node pools are ready"
	case ConditionStatusError:
		return "One or more node pools have errors"
	case ConditionStatusWorking:
		return "Node pools are being provisioned"
	default:
		return "Node pools status unknown"
	}
}
