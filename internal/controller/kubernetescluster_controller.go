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

	"time"

	"github.com/vitistack/common/pkg/loggers/vlog"
	vitistackv1alpha1 "github.com/vitistack/common/pkg/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	KubernetesClusterFinalizer               = "kubernetescluster.vitistack.io/finalizer"
	ControllerRequeueDelay     time.Duration = 5 * time.Second
)

// KubernetesClusterReconciler reconciles a KubernetesCluster object
type KubernetesClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vitistack.io,resources=kubernetesclusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the KubernetesCluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *KubernetesClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// Fetch the KubernetesCluster
	kubernetesCluster := &vitistackv1alpha1.KubernetesCluster{}
	if err := r.Get(ctx, req.NamespacedName, kubernetesCluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion first and only perform cleanup when our finalizer is present
	// if kubernetesCluster.GetDeletionTimestamp() != nil {
	// 	return r.handleDeletion(ctx, kubernetesCluster)
	// }

	// Only reconcile clusters with spec.cluster.provider == "aks"
	if !r.isAKSProvider(kubernetesCluster) {
		vlog.Info("Skipping KubernetesCluster: unsupported provider (need 'aks'): cluster=" + kubernetesCluster.GetName())
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubernetesClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		// For().
		Named("kubernetescluster").
		Complete(r)
}

func (r *KubernetesClusterReconciler) isAKSProvider(kubernetesCluster *vitistackv1alpha1.KubernetesCluster) bool {
	return kubernetesCluster.Spec.Cluster.Provider == vitistackv1alpha1.KubernetesProviderTypeAKS
}
