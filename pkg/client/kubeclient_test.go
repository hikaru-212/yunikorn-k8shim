/*
 Licensed to the Apache Software Foundation (ASF) under one
 or more contributor license agreements.  See the NOTICE file
 distributed with this work for additional information
 regarding copyright ownership.  The ASF licenses this file
 to you under the Apache License, Version 2.0 (the
 "License"); you may not use this file except in compliance
 with the License.  You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package client

import (
	"context"
	"errors"
	"testing"

	"gotest.tools/v3/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSchedulerKubeClientDeleteUIDPrecondition(t *testing.T) {
	const (
		namespace = "default"
		podName   = "victim"
	)
	oldUID := types.UID("uid-a")
	replacementUID := types.UID("uid-b")
	oldPod := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace,
		Name:      podName,
		UID:       oldUID,
	}}
	replacement := &v1.Pod{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace,
		Name:      podName,
		UID:       replacementUID,
	}}

	clientSet := fake.NewSimpleClientset(replacement)
	var deleteOptions metav1.DeleteOptions
	clientSet.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		deleteAction := action.(k8stesting.DeleteAction) //nolint:errcheck
		deleteOptions = deleteAction.GetDeleteOptions()
		if deleteOptions.Preconditions == nil || deleteOptions.Preconditions.UID == nil ||
			*deleteOptions.Preconditions.UID != replacementUID {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Resource: "pods"}, podName, errors.New("UID precondition does not match"))
		}
		return false, nil, nil
	})

	kubeClient := SchedulerKubeClient{clientSet: clientSet}
	err := kubeClient.Delete(oldPod)
	assert.ErrorContains(t, err, "UID precondition does not match")
	assert.Assert(t, deleteOptions.Preconditions != nil)
	assert.Assert(t, deleteOptions.Preconditions.UID != nil)
	assert.Equal(t, oldUID, *deleteOptions.Preconditions.UID,
		"DELETE must carry the authority of the original pod UID")

	remaining, err := clientSet.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	assert.NilError(t, err, "replacement pod must remain after stale delete")
	assert.Equal(t, replacementUID, remaining.UID, "stale delete removed the replacement pod")

	clientSet = fake.NewSimpleClientset()
	kubeClient = SchedulerKubeClient{clientSet: clientSet}
	err = kubeClient.Delete(&v1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: podName}})
	assert.ErrorContains(t, err, "without a UID")
	assert.Equal(t, 0, len(clientSet.Actions()), "empty UID must not issue an unfenced DELETE")
}
