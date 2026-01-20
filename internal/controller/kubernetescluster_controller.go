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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/aks-operator/internal/services/clusterstate"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

	// Verify cluster state service is initialized (it should be from main.go)
	if r.ClusterStateService == nil {
		vlog.Error(nil, "ClusterStateService not initialized - this should not happen")
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, fmt.Errorf("cluster state service not initialized")
	}

	// IMPORTANT: Check for deletion FIRST, before any other operations
	// This ensures we handle deletion even if secret creation fails
	if kubernetesCluster.GetDeletionTimestamp() != nil {
		vlog.Info("🗑️ Deletion detected", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		return r.handleDeletion(ctx, kubernetesCluster)
	}

	// Ensure state secret exists (only for non-deleted clusters)
	_, err := r.ClusterStateService.GetOrCreateState(ctx, kubernetesCluster)
	if err != nil {
		vlog.Error(err, "failed to get or create cluster state secret", "cluster", kubernetesCluster.Name)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(kubernetesCluster, KubernetesClusterFinalizer) {
		vlog.Info("🔒 Adding finalizer", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		controllerutil.AddFinalizer(kubernetesCluster, KubernetesClusterFinalizer)
		if err := r.Update(ctx, kubernetesCluster); err != nil {
			vlog.Error(err, "failed to add finalizer", "cluster", kubernetesCluster.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Set initial status to Creating if phase is empty (new cluster)
	if kubernetesCluster.Status.Phase == "" {
		vlog.Info("🆕 New cluster", "cluster", kubernetesCluster.Name, "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		if err := r.updatePhaseAndCondition(ctx, kubernetesCluster, PhaseCreating, ConditionClusterProvisioned, ConditionStatusWorking, "Cluster creation initiated"); err != nil {
			vlog.Error(err, "failed to set initial status", "cluster", kubernetesCluster.Name)
		}
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
			// Only log phase transitions
			if currentPhase != "" && currentPhase != phase {
				vlog.Info("📦 Phase transition", "cluster", kubernetesCluster.Name, "from", currentPhase, "to", phase, "aksState", aksState)
			}

			resourceGroupName := kubernetesCluster.Spec.Cluster.Project
			clusterName := kubernetesCluster.Spec.Cluster.ClusterId

			if err := r.ClusterStateService.SetClusterProvisioned(ctx, kubernetesCluster, *aksCluster.ID, clusterName, resourceGroupName); err != nil {
				vlog.Error(err, "failed to update cluster state", "cluster", kubernetesCluster.Name)
			}

			// Store FQDN if available
			if aksCluster.Properties.Fqdn != nil {
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
				vlog.Info("🔑 Kubeconfig stored", "cluster", kubernetesCluster.Name)
				if err := r.ClusterStateService.SetKubernetesAPIReady(ctx, kubernetesCluster); err != nil {
					vlog.Error(err, "failed to update Kubernetes API state", "cluster", kubernetesCluster.Name)
				}
				if err := r.ClusterStateService.SetClusterReady(ctx, kubernetesCluster); err != nil {
					vlog.Error(err, "failed to mark cluster as ready", "cluster", kubernetesCluster.Name)
				}
			}

			// Fetch and store kube-system UID if not already stored
			if _, err := r.fetchAndStoreKubeSystemUID(ctx, kubernetesCluster); err != nil {
				vlog.Error(err, "failed to fetch kube-system UID", "cluster", kubernetesCluster.Name)
			}
		}

		// Update the cluster status only if phase changed
		if needsStateUpdate {
			if err := r.updateClusterStatus(ctx, kubernetesCluster, aksCluster, phase); err != nil {
				vlog.Error(err, "failed to update cluster status", "cluster", kubernetesCluster.Name)
			}
		}
	}

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

// handleDeletion handles the deletion of an AKS cluster
func (r *KubernetesClusterReconciler) handleDeletion(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (ctrl.Result, error) {
	// Even if finalizer is not present, we should still try to delete the Azure resources
	// to handle cases where the CR was created but finalizer wasn't added yet
	hasFinalizer := controllerutil.ContainsFinalizer(kubernetesCluster, KubernetesClusterFinalizer)

	vlog.Info("🗑️ Starting AKS cluster deletion",
		"cluster", kubernetesCluster.Name,
		"namespace", kubernetesCluster.Namespace,
		"clusterid", kubernetesCluster.Spec.Cluster.ClusterId,
		"hasFinalizer", hasFinalizer,
		"resourceGroup", kubernetesCluster.Spec.Cluster.Project)

	// Update status (best effort - may fail if CR is already being deleted)
	if err := r.updatePhaseAndCondition(ctx, kubernetesCluster, PhaseDeleting, ConditionClusterReady, ConditionStatusWorking, "Deleting AKS cluster"); err != nil {
		vlog.Error(err, "failed to update status (expected during deletion)", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	}

	// Delete the AKS cluster from Azure
	vlog.Info("🔥 Calling Azure to delete AKS cluster",
		"cluster", kubernetesCluster.Name,
		"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	if err := r.AKSClusterManager.DeleteCluster(ctx, kubernetesCluster); err != nil {
		vlog.Error(err, "❌ Failed to delete AKS cluster from Azure", "clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, err
	}

	vlog.Info("✅ AKS cluster deleted from Azure",
		"cluster", kubernetesCluster.Name,
		"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)

	// Delete the state secret (it should be auto-deleted due to owner reference, but ensure cleanup)
	if err := r.ClusterStateService.DeleteState(ctx, kubernetesCluster); err != nil {
		vlog.Error(err, "failed to delete cluster state secret (may already be deleted)", "cluster", kubernetesCluster.Name)
	}

	// Remove finalizer if present
	if hasFinalizer {
		vlog.Info("🔓 Removing finalizer", "cluster", kubernetesCluster.Name)
		controllerutil.RemoveFinalizer(kubernetesCluster, KubernetesClusterFinalizer)
		if err := r.Update(ctx, kubernetesCluster); err != nil {
			vlog.Error(err, "failed to remove finalizer", "cluster", kubernetesCluster.Name)
			return ctrl.Result{}, err
		}
	}

	vlog.Info("✅ AKS cluster deletion completed successfully",
		"cluster", kubernetesCluster.Name,
		"clusterid", kubernetesCluster.Spec.Cluster.ClusterId)
	return ctrl.Result{}, nil
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
