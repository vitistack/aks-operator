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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice/v6"
	"github.com/vitistack/aks-operator/pkg/interfaces"
	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	KubernetesClusterFinalizer               = "kubernetescluster.vitistack.io/finalizer"
	ControllerRequeueDelay     time.Duration = 30 * time.Second
	ControllerRequeueError     time.Duration = 60 * time.Second

	// Cluster phase constants
	phaseRunning      = "Running"
	phaseFailed       = "Failed"
	phaseProvisioning = "Provisioning"
	phaseDeleting     = "Deleting"
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
}

// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=vitistack.io,resources=machineclasses,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *KubernetesClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// Fetch the KubernetesCluster
	kubernetesCluster := &vitistackv1alpha1.KubernetesCluster{}
	if err := r.Get(ctx, req.NamespacedName, kubernetesCluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Only reconcile clusters with spec.cluster.provider == "aks"
	if !r.isAKSProvider(kubernetesCluster) {
		vlog.Info("Skipping KubernetesCluster: unsupported provider (need 'aks')", "cluster", kubernetesCluster.GetName())
		return ctrl.Result{}, nil
	}

	// Verify Azure services are initialized (they should be from main.go)
	if r.AKSClusterManager == nil {
		vlog.Error(nil, "Azure services not initialized - this should not happen")
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, fmt.Errorf("azure services not initialized")
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
	vlog.Info("Reconciling AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	// Update status to show we're working on it
	if err := r.updateStatus(ctx, kubernetesCluster, "Provisioning", "Reconciling cluster"); err != nil {
		vlog.Error(err, "failed to update status")
	}

	// Reconcile the AKS cluster
	aksCluster, err := r.AKSClusterManager.ReconcileCluster(ctx, kubernetesCluster)
	if err != nil {
		if statusErr := r.updateStatus(ctx, kubernetesCluster, "Failed", err.Error()); statusErr != nil {
			vlog.Error(statusErr, "failed to update status")
		}
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, err
	}

	// Update status with cluster information
	if aksCluster != nil && aksCluster.Properties != nil {
		phase := phaseRunning
		if aksCluster.Properties.ProvisioningState != nil {
			provisioningState := *aksCluster.Properties.ProvisioningState
			switch provisioningState {
			case "Succeeded":
				phase = phaseRunning
			case "Creating", "Updating":
				phase = phaseProvisioning
			case "Deleting":
				phase = phaseDeleting
			case "Failed":
				phase = phaseFailed
			default:
				phase = provisioningState
			}
		}

		// Update the cluster status
		if err := r.updateClusterStatus(ctx, kubernetesCluster, aksCluster, phase); err != nil {
			vlog.Error(err, "failed to update cluster status")
		}
	}

	vlog.Info("AKS cluster reconciled successfully", "cluster", kubernetesCluster.Name)
	return ctrl.Result{RequeueAfter: ControllerRequeueDelay}, nil
}

// handleDeletion handles the deletion of an AKS cluster
func (r *KubernetesClusterReconciler) handleDeletion(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(kubernetesCluster, KubernetesClusterFinalizer) {
		return ctrl.Result{}, nil
	}

	vlog.Info("Deleting AKS cluster", "cluster", kubernetesCluster.Name, "namespace", kubernetesCluster.Namespace)

	// Update status
	if err := r.updateStatus(ctx, kubernetesCluster, "Deleting", "Deleting AKS cluster"); err != nil {
		vlog.Error(err, "failed to update status")
	}

	// Delete the AKS cluster
	if err := r.AKSClusterManager.DeleteCluster(ctx, kubernetesCluster); err != nil {
		vlog.Error(err, "failed to delete AKS cluster")
		return ctrl.Result{RequeueAfter: ControllerRequeueError}, err
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(kubernetesCluster, KubernetesClusterFinalizer)
	if err := r.Update(ctx, kubernetesCluster); err != nil {
		return ctrl.Result{}, err
	}

	vlog.Info("AKS cluster deleted successfully", "cluster", kubernetesCluster.Name)
	return ctrl.Result{}, nil
}

// updateStatus updates the status phase and message
func (r *KubernetesClusterReconciler) updateStatus(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, phase, message string) error {
	kubernetesCluster.Status.Phase = phase

	// Add or update condition
	conditionType := "ClusterReady"
	var status string
	switch phase {
	case phaseRunning:
		status = "ok"
	case phaseFailed:
		status = "error"
	default:
		status = "working"
	}

	// Update or add condition
	found := false
	for i := range kubernetesCluster.Status.Conditions {
		if kubernetesCluster.Status.Conditions[i].Type == conditionType {
			kubernetesCluster.Status.Conditions[i].Status = status
			kubernetesCluster.Status.Conditions[i].Message = message
			kubernetesCluster.Status.Conditions[i].LastTransitionTime = time.Now().Format(time.RFC3339)
			found = true
			break
		}
	}
	if !found {
		kubernetesCluster.Status.Conditions = append(kubernetesCluster.Status.Conditions, vitistackv1alpha1.KubernetesClusterCondition{
			Type:               conditionType,
			Status:             status,
			Message:            message,
			LastTransitionTime: time.Now().Format(time.RFC3339),
		})
	}

	return r.Status().Update(ctx, kubernetesCluster)
}

// updateClusterStatus updates the full cluster status from AKS cluster data
func (r *KubernetesClusterReconciler) updateClusterStatus(ctx context.Context, kubernetesCluster *vitistackv1alpha1.KubernetesCluster, aksCluster *armcontainerservice.ManagedCluster, phase string) error {
	// Store the AKS cluster ID in annotations
	if aksCluster.ID != nil {
		if kubernetesCluster.Annotations == nil {
			kubernetesCluster.Annotations = make(map[string]string)
		}
		kubernetesCluster.Annotations["vitistack.io/aks-cluster-id"] = *aksCluster.ID

		// Update the resource with the new annotation
		if err := r.Update(ctx, kubernetesCluster); err != nil {
			return fmt.Errorf("failed to update annotations: %w", err)
		}
	}

	// Build status message from provisioning state
	message := "Cluster is " + phase
	if aksCluster.Properties != nil && aksCluster.Properties.ProvisioningState != nil {
		message = fmt.Sprintf("Cluster is %s (Azure state: %s)", phase, *aksCluster.Properties.ProvisioningState)
	}

	return r.updateStatus(ctx, kubernetesCluster, phase, message)
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vitistackv1alpha1.KubernetesCluster{}).
		Named("kubernetescluster").
		Complete(r)
}

func (r *KubernetesClusterReconciler) isAKSProvider(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) bool {
	return kubernetesCluster.Spec.Cluster.Provider == vitistackv1alpha1.KubernetesProviderTypeAKS
}
