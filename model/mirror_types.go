/*
Copyright 2021.

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

package model

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Mirror defines the desired state of Mirror
type Mirror struct {
	Name   string           `json:"name,omitempty"`
	Config MirrorSyncConfig `json:"config,omitempty"`
	// +kubebuilder:validation:Required
	Resources []MirrorSyncTarget    `json:"resources,omitempty"`
	Selector  *metav1.LabelSelector `json:"selector,omitempty"`
	Filter    []MirrorAction        `json:"filter,omitempty"`
}

type MirrorCluster struct {
	// +kubebuilder:validation:Required
	Main string `json:"master,omitempty"`
	// +kubebuilder:validation:MinItems:=1
	Follower []string `json:"follower,omitempty"`
}

type MirrorSyncConfig struct {
	// +kubebuilder:validation:Required
	Clusters            MirrorCluster   `json:"clusters,omitempty"`
	Namespace           string          `json:"namespace,omitempty"`
	NotInNamespace      string          `json:"notInNamespace,omitempty"`
	RsyncPeriodDuration metav1.Duration `json:"rsyncPeriodDuration,omitempty"`
	TargetName          string          `json:"targetName,omitempty"`
	// create if target not exists
	SyncCreate bool `json:"syncCreate,omitempty"`
	// delete if source is delete
	SyncDelete bool `json:"syncDelete,omitempty"`
}

type MirrorSyncTarget struct {
	Version string `json:"version,omitempty"`
	Group   string `json:"group,omitempty"`
	Kind    string `json:"kind,omitempty"`
}

type MirrorAction struct {
	Action string `json:"action,omitempty"`
	Key    string `json:"key,omitempty"`
	Value  string `json:"value,omitempty"`
}
