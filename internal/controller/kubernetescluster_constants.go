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

import "time"

const (
	// KubernetesClusterFinalizer is the finalizer used for KubernetesCluster resources
	KubernetesClusterFinalizer = "kubernetescluster.vitistack.io/finalizer"

	// Controller requeue delays
	ControllerRequeueDelay time.Duration = 5 * time.Second
	ControllerRequeueError time.Duration = 60 * time.Second

	// Cluster phase constants
	PhaseCreating     = "Creating"
	PhaseProvisioning = "Provisioning"
	PhaseReady        = "Ready"
	PhaseUpdating     = "Updating"
	PhaseDeleting     = "Deleting"
	PhaseFailed       = "Failed"

	// Condition types
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

	// Annotations
	AnnotationAKSClusterID  = "vitistack.io/aks-cluster-id"
	AnnotationKubeSystemUID = "vitistack.io/kube-system-uid"
)
