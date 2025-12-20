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
	"github.com/vitistack/aks-operator/internal/services/clusterstate"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	KubernetesClusterFinalizer               = "kubernetescluster.vitistack.io/finalizer"
	ControllerRequeueDelay     time.Duration = 5 * time.Second
	ControllerRequeueError     time.Duration = 60 * time.Second

	// Cluster phase constants
	PhaseCreating     = "Creating"
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseUpdating     = "Updating"
	PhaseDeleting     = "Deleting"
	PhaseFailed       = "Failed"

	// Condition types (sorted alphabetically for consistent ordering)
	ConditionClusterProvisioned = "ClusterProvisioned"
	ConditionClusterReady       = "ClusterReady"
	ConditionKubeConfigReady    = "KubeConfigReady"
	ConditionKubernetesAPIReady = "KubernetesAPIReady"
	ConditionNodePoolsReady     = "NodePoolsReady"

	// Condition statuses
	ConditionStatusOK      = "ok"
	ConditionStatusError   = "error"
	ConditionStatusWarning = "warning"
	ConditionStatusWorking = "working"
	ConditionStatusUnknown = "unknown"
)

// KubernetesClusterReconciler reconciles a KubernetesCluster object
type KubernetesClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Azure clients and managers
	AzureClientFactory interfaces.AzureClientFactory
	AKSClusterManager  interfaces.AKSClusterManager
	AgentPoolManager   interfaces.AgentPoolManager
	VMSizeManager      interfaces.VMSizeManager
	MachineClassMapper interfaces.MachineClassMapper

	// Cluster state service
	ClusterStateService *clusterstate.ClusterStateService
}

// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=vitistack.io,resources=machineclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KubernetesClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// Fetch the KubernetesCluster
	kubernetesCluster := &vitistackv1alpha1.KubernetesCluster{}
	if err := r.Get(ctx, req.NamespacedName, kubernetesCluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Verify Azure services are initialized (they should be from main.go)
	if r.AKSClusterManager == nil {
		vlog.Error(nil, "Azure services not initialized - this should not happen")
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, fmt.Errorf("azure services not initialized")
	}

	// Initialize cluster state service if not set
	if r.ClusterStateService == nil {
		r.ClusterStateService = clusterstate.NewClusterStateService(r.Client)
	}

	// Ensure state secret exists
	_, err := r.ClusterStateService.GetOrCreateState(ctx, kubernetesCluster)
	if err != nil {
		vlog.Error(err, "failed to get or create cluster state secret", "cluster", kubernetesCluster.Name)
	}

	// Handle deletion
	if kubernetesCluster.GetDeletionTimestamp() != nil {
		return r.handleDeletion(ctx, kubernetesCluster)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kubernetesCluster, KubernetesClusterFinalizer) {
		controllerutil.AddFinalizer(kubernetesCluster, KubernetesClusterFinalizer)
		if err := r.Update(ctx, kubernetesCluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile the AKS cluster
	return r.reconcileCluster(ctx, kubernetesCluster)
}

// reconcileCluster handles the reconciliation of an AKS cluster
func (r *KubernetesClusterReconciler) reconcileCluster(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (ctrl.Result, error) {
	// Reconcile the AKS cluster - this will create or update as needed
	aksCluster, err := r.AKSClusterManager.ReconcileCluster(ctx, kubernetesCluster)
	if err != nil {
		vlog.Error(err, "❌ Failed to reconcile AKS cluster", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		// Update state with failure
		if stateErr := r.ClusterStateService.SetClusterFailed(ctx, kubernetesCluster, err.Error()); stateErr != nil {
			vlog.Error(stateErr, "failed to update cluster state", "cluster", kubernetesCluster.Name)
		}

		if statusErr := r.updatePhaseAndCondition(ctx, kubernetesCluster, PhaseFailed, ConditionClusterProvisioned, ConditionStatusError, err.Error()); statusErr != nil {
			vlog.Error(statusErr, "failed to update status", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		}
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, err
	}

	// Update status with cluster information
	if aksCluster != nil && aksCluster.Properties != nil {
		phase, aksState := r.determinePhaseFromAKS(aksCluster)

		// Check if we need to update state (only on first provision or state change)
		currentPhase := kubernetesCluster.Status.Phase
		needsStateUpdate := currentPhase != phase || currentPhase == "" || currentPhase == PhaseCreating || currentPhase == PhaseProvisioning

		// Update state secret with AKS info only when needed
		if needsStateUpdate && aksCluster.ID != nil {
			vlog.Info("📦 Phase changed, updating cluster state",
				"cluster", kubernetesCluster.Name,
				"previousPhase", currentPhase,
				"newPhase", phase,
				"aksState", aksState,
				"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

			resourceGroupName := kubernetesCluster.Spec.Cluster.Project
			clusterName := kubernetesCluster.Spec.Cluster.ClusterId

			if err := r.ClusterStateService.SetClusterProvisioned(ctx, kubernetesCluster, *aksCluster.ID, clusterName, resourceGroupName); err != nil {
				vlog.Error(err, "failed to update cluster state", "cluster", kubernetesCluster.Name)
			}

			// Store FQDN if available
			if aksCluster.Properties.Fqdn != nil {
				vlog.Info("🌐 Storing cluster FQDN", "cluster", kubernetesCluster.Name, "fqdn", *aksCluster.Properties.Fqdn)
				if err := r.ClusterStateService.SetClusterFQDN(ctx, kubernetesCluster, *aksCluster.Properties.Fqdn); err != nil {
					vlog.Error(err, "failed to update cluster FQDN state", "cluster", kubernetesCluster.Name)
				}
			}

			// Update AKS provision state
			if aksCluster.Properties.ProvisioningState != nil {
				if err := r.ClusterStateService.SetAKSProvisionState(ctx, kubernetesCluster, *aksCluster.Properties.ProvisioningState); err != nil {
					vlog.Error(err, "failed to update AKS provision state", "cluster", kubernetesCluster.Name)
				}
			}
		}

		// If cluster is running, fetch and store kubeconfig (only if not already stored)
		if phase == PhaseReady || aksState == "Succeeded" {
			kubeconfigUpdated, err := r.fetchAndStoreKubeconfig(ctx, kubernetesCluster)
			if err != nil {
				vlog.Error(err, "failed to fetch kubeconfig", "cluster", kubernetesCluster.Name)
			} else if kubeconfigUpdated {
				vlog.Info("🔑 Kubeconfig stored successfully", "cluster", kubernetesCluster.Name)
				// Only update these states when kubeconfig is first stored
				if err := r.ClusterStateService.SetKubernetesAPIReady(ctx, kubernetesCluster); err != nil {
					vlog.Error(err, "failed to update Kubernetes API state", "cluster", kubernetesCluster.Name)
				}
				if err := r.ClusterStateService.SetClusterReady(ctx, kubernetesCluster); err != nil {
					vlog.Error(err, "failed to mark cluster as ready", "cluster", kubernetesCluster.Name)
				}
			}
		}

		// Update the cluster status only if phase changed
		if needsStateUpdate {
			vlog.Info("✅ Cluster status updated",
				"cluster", kubernetesCluster.Name,
				"phase", phase,
				"aksState", aksState,
				"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			if err := r.updateClusterStatus(ctx, kubernetesCluster, aksCluster, phase); err != nil {
				vlog.Error(err, "failed to update cluster status", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
			}
		}
	}

	// No logging when nothing changed - silence is golden
	return ctrl.Result{RequeueAfter: ControllerRequeueDelay}, nil
}

// determinePhaseFromAKS determines the cluster phase from AKS provisioning state
func (r *KubernetesClusterReconciler) determinePhaseFromAKS(aksCluster *armcontainerservice.ManagedCluster) (string, string) {
	if aksCluster.Properties == nil || aksCluster.Properties.ProvisioningState == nil {
		return PhaseProvisioning, "Unknown"
	}

	provisioningState := *aksCluster.Properties.ProvisioningState
	var phase string

	switch provisioningState {
	case "Succeeded":
		phase = PhaseReady
	case "Creating":
		phase = PhaseCreating
	case "Updating":
		phase = PhaseUpdating
	case "Deleting":
		phase = PhaseDeleting
	case "Failed":
		phase = PhaseFailed
	default:
		phase = PhaseProvisioning
	}

	return phase, provisioningState
}

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

// handleDeletion handles the deletion of an AKS cluster
func (r *KubernetesClusterReconciler) handleDeletion(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kubernetesCluster, KubernetesClusterFinalizer) {
		return ctrl.Result{}, nil
	}

	vlog.Info("Deleting AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	// Update status
	if err := r.updatePhaseAndCondition(ctx, kubernetesCluster, PhaseDeleting, ConditionClusterReady, ConditionStatusWorking, "Deleting AKS cluster"); err != nil {
		vlog.Error(err, "failed to update status", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	}

	// Delete the AKS cluster
	if err := r.AKSClusterManager.DeleteCluster(ctx, kubernetesCluster); err != nil {
		vlog.Error(err, "failed to delete AKS cluster", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, err
	}

	// Delete the state secret (it should be auto-deleted due to owner reference, but ensure cleanup)
	if err := r.ClusterStateService.DeleteState(ctx, kubernetesCluster); err != nil {
		vlog.Error(err, "failed to delete cluster state secret", "cluster", kubernetesCluster.Name)
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(kubernetesCluster, KubernetesClusterFinalizer)
	if err := r.Update(ctx, kubernetesCluster); err != nil {
		return ctrl.Result{}, err
	}

	vlog.Info("AKS cluster deleted successfully", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return ctrl.Result{}, nil
}

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

	// Sort conditions alphabetically by type
	r.sortConditions(kubernetesCluster)

	return r.Status().Update(ctx, kubernetesCluster)
}

// setCondition updates or adds a condition
func (r *KubernetesClusterReconciler) setCondition(kubernetesCluster *vitistackv1alpha1.KubernetesCluster, conditionType, status, message string) {
	now := time.Now().Format(time.RFC3339)

	// Find existing condition
	found := false
	for i := range kubernetesCluster.Status.Conditions {
		if kubernetesCluster.Status.Conditions[i].Type == conditionType {
			kubernetesCluster.Status.Conditions[i].Status = status
			kubernetesCluster.Status.Conditions[i].Message = message
			kubernetesCluster.Status.Conditions[i].LastTransitionTime = now
			found = true
			break
		}
	}

	if !found {
		kubernetesCluster.Status.Conditions = append(kubernetesCluster.Status.Conditions, vitistackv1alpha1.KubernetesClusterCondition{
			Type:               conditionType,
			Status:             status,
			Message:            message,
			LastTransitionTime: now,
		})
	}
}

// sortConditions sorts conditions alphabetically by type
func (r *KubernetesClusterReconciler) sortConditions(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) {
	sort.Slice(kubernetesCluster.Status.Conditions, func(i, j int) bool {
		return kubernetesCluster.Status.Conditions[i].Type < kubernetesCluster.Status.Conditions[j].Type
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
		if kubernetesCluster.Annotations["vitistack.io/aks-cluster-id"] != *aksCluster.ID {
			kubernetesCluster.Annotations["vitistack.io/aks-cluster-id"] = *aksCluster.ID
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
			vlog.Info("Status update conflict, retrying...", "cluster", kubernetesCluster.Name)
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

	// Set all conditions
	r.setCondition(kubernetesCluster, ConditionClusterProvisioned, provisionedStatus, fmt.Sprintf("Azure state: %s", provisioningState))
	r.setCondition(kubernetesCluster, ConditionClusterReady, readyStatus, message)
	r.setCondition(kubernetesCluster, ConditionKubernetesAPIReady, apiReadyStatus, getAPIReadyMessage(apiReadyStatus))
	r.setCondition(kubernetesCluster, ConditionKubeConfigReady, kubeconfigStatus, getKubeconfigMessage(kubeconfigStatus))
	r.setCondition(kubernetesCluster, ConditionNodePoolsReady, nodepoolsStatus, getNodePoolsMessage(nodepoolsStatus))
}

// Helper functions for condition messages
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

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Filter to only process KubernetesClusters with provider == "aks"
	aksProviderPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*vitistackv1alpha1.KubernetesCluster)
			if !ok {
				return false
			}
			return strings.EqualFold(string(cluster.Spec.Cluster.Provider), string(vitistackv1alpha1.KubernetesProviderTypeAKS))
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			cluster, ok := e.ObjectNew.(*vitistackv1alpha1.KubernetesCluster)
			if !ok {
				return false
			}
			return strings.EqualFold(string(cluster.Spec.Cluster.Provider), string(vitistackv1alpha1.KubernetesProviderTypeAKS))
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			cluster, ok := e.Object.(*vitistackv1alpha1.KubernetesCluster)
			if !ok {
				return false
			}
			return strings.EqualFold(string(cluster.Spec.Cluster.Provider), string(vitistackv1alpha1.KubernetesProviderTypeAKS))
		},
		GenericFunc: func(e event.GenericEvent) bool {
			cluster, ok := e.Object.(*vitistackv1alpha1.KubernetesCluster)
			if !ok {
				return false
			}
			return strings.EqualFold(string(cluster.Spec.Cluster.Provider), string(vitistackv1alpha1.KubernetesProviderTypeAKS))
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&vitistackv1alpha1.KubernetesCluster{}).
		WithEventFilter(aksProviderPredicate).
		Named("kubernetescluster").
		Complete(r)
}
