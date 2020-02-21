/*
Copyright 2018 The Volcano Authors.

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

package mutate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	schedulingv1beta1 "volcano.sh/volcano/pkg/apis/scheduling/v1beta1"
	"volcano.sh/volcano/pkg/webhooks/util"
)

func TestMutateQueues(t *testing.T) {
	trueValue := true
	stateNotSetReclaimableNotSet := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-case-refresh-default-state",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
	}

	stateNotSetJSON, err := json.Marshal(stateNotSetReclaimableNotSet)
	if err != nil {
		t.Errorf("Marshal queue without state set failed for %v.", err)
	}

	openStateReclaimableSet := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-case-set-open",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight:      1,
			Reclaimable: &trueValue,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: schedulingv1beta1.QueueStateOpen,
		},
	}

	openStateJSON, err := json.Marshal(openStateReclaimableSet)
	if err != nil {
		t.Errorf("Marshal queue with open state failed for %v.", err)
	}

	pt := v1beta1.PatchTypeJSONPatch

	var refreshPatch []patchOperation
	refreshPatch = append(refreshPatch, patchOperation{
		Op:    "add",
		Path:  "/spec/state",
		Value: schedulingv1beta1.QueueStateOpen,
	}, patchOperation{
		Op:    "add",
		Path:  "/spec/reclaimable",
		Value: &trueValue,
	})

	refreshPatchJSON, err := json.Marshal(refreshPatch)
	if err != nil {
		t.Errorf("Marshal queue patch failed for %v.", err)
	}

	var openStatePatch []patchOperation
	openStatePatch = append(openStatePatch, patchOperation{
		Op:    "add",
		Path:  "/spec/state",
		Value: schedulingv1beta1.QueueStateOpen,
	})
	openStatePatchJSON, err := json.Marshal(openStatePatch)
	if err != nil {
		t.Errorf("Marshal null patch failed for %v.", err)
	}

	testCases := []struct {
		Name           string
		AR             v1beta1.AdmissionReview
		reviewResponse *v1beta1.AdmissionResponse
	}{
		{
			Name: "Normal Case Refresh Default Open State and Reclaimable For Queue",
			AR: v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Request: &v1beta1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "scheduling.volcano.sh",
						Version: "v1beta1",
						Kind:    "Queue",
					},
					Resource: metav1.GroupVersionResource{
						Group:    "scheduling.volcano.sh",
						Version:  "v1beta1",
						Resource: "queues",
					},
					Name:      "normal-case-refresh-default-state",
					Operation: "CREATE",
					Object: runtime.RawExtension{
						Raw: stateNotSetJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed:   true,
				PatchType: &pt,
				Patch:     refreshPatchJSON,
			},
		},
		{
			Name: "Normal Case Without Queue State or Reclaimable Patch",
			AR: v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Request: &v1beta1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "scheduling.volcano.sh",
						Version: "v1beta1",
						Kind:    "Queue",
					},
					Resource: metav1.GroupVersionResource{
						Group:    "scheduling.volcano.sh",
						Version:  "v1beta1",
						Resource: "queues",
					},
					Name:      "normal-case-set-open",
					Operation: "CREATE",
					Object: runtime.RawExtension{
						Raw: openStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed:   true,
				PatchType: &pt,
				Patch:     openStatePatchJSON,
			},
		},
		{
			Name: "Invalid Action",
			AR: v1beta1.AdmissionReview{
				TypeMeta: metav1.TypeMeta{
					Kind:       "AdmissionReview",
					APIVersion: "admission.k8s.io/v1beta1",
				},
				Request: &v1beta1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "scheduling.volcano.sh",
						Version: "v1beta1",
						Kind:    "Queue",
					},
					Resource: metav1.GroupVersionResource{
						Group:    "scheduling.volcano.sh",
						Version:  "v1beta1",
						Resource: "queues",
					},
					Name:      "normal-case-set-open",
					Operation: "Invalid",
					Object: runtime.RawExtension{
						Raw: openStateJSON,
					},
				},
			},
			reviewResponse: util.ToAdmissionResponse(fmt.Errorf("invalid operation `%s`, "+
				"expect operation to be `CREATE`", "Invalid")),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			reviewResponse := MutateQueues(testCase.AR)
			if !reflect.DeepEqual(reviewResponse, testCase.reviewResponse) {
				t.Errorf("Test case '%s' failed, expect: %v, got: %v", testCase.Name,
					reviewResponse, testCase.reviewResponse)
			}
		})
	}
}
