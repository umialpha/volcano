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

package validate

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"

	schedulingv1beta1 "volcano.sh/volcano/pkg/apis/scheduling/v1beta1"
	fakeclient "volcano.sh/volcano/pkg/client/clientset/versioned/fake"
	"volcano.sh/volcano/pkg/webhooks/util"
)

func TestAdmitQueues(t *testing.T) {
	stateNotSet := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-case-not-set",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
	}

	stateNotSetJSON, err := json.Marshal(stateNotSet)
	if err != nil {
		t.Errorf("Marshal queue without state set failed for %v.", err)
	}

	openState := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-case-set-open",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: schedulingv1beta1.QueueStateOpen,
		},
	}

	openStateJSON, err := json.Marshal(openState)
	if err != nil {
		t.Errorf("Marshal queue with open state failed for %v.", err)
	}

	closedState := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "normal-case-set-closed",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: schedulingv1beta1.QueueStateClosed,
		},
	}

	closedStateJSON, err := json.Marshal(closedState)
	if err != nil {
		t.Errorf("Marshal queue with closed state failed for %v.", err)
	}

	wrongState := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "abnormal-case",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: "wrong",
		},
	}

	wrongStateJSON, err := json.Marshal(wrongState)
	if err != nil {
		t.Errorf("Marshal queue with wrong state failed for %v.", err)
	}

	openStateForDelete := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "open-state-for-delete",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: schedulingv1beta1.QueueStateOpen,
		},
	}

	openStateForDeleteJSON, err := json.Marshal(openStateForDelete)
	if err != nil {
		t.Errorf("Marshal queue for delete with open state failed for %v.", err)
	}

	closedStateForDelete := schedulingv1beta1.Queue{
		ObjectMeta: metav1.ObjectMeta{
			Name: "closed-state-for-delete",
		},
		Spec: schedulingv1beta1.QueueSpec{
			Weight: 1,
		},
		Status: schedulingv1beta1.QueueStatus{
			State: schedulingv1beta1.QueueStateClosed,
		},
	}

	closedStateForDeleteJSON, err := json.Marshal(closedStateForDelete)
	if err != nil {
		t.Errorf("Marshal queue for delete with closed state failed for %v.", err)
	}

	config.VolcanoClient = fakeclient.NewSimpleClientset()
	_, err = config.VolcanoClient.SchedulingV1beta1().Queues().Create(&openStateForDelete)
	if err != nil {
		t.Errorf("Crate queue with open state failed for %v.", err)
	}

	_, err = config.VolcanoClient.SchedulingV1beta1().Queues().Create(&closedStateForDelete)
	if err != nil {
		t.Errorf("Crate queue with closed state failed for %v.", err)
	}

	defer func() {
		if err := config.VolcanoClient.SchedulingV1beta1().Queues().Delete(openStateForDelete.Name, &v1.DeleteOptions{}); err != nil {
			fmt.Println(fmt.Sprintf("Delete queue with open state failed for %v.", err))
		}
		if err := config.VolcanoClient.SchedulingV1beta1().Queues().Delete(closedStateForDelete.Name, &v1.DeleteOptions{}); err != nil {
			fmt.Println(fmt.Sprintf("Delete queue with closed state failed for %v.", err))
		}
	}()

	testCases := []struct {
		Name           string
		AR             v1beta1.AdmissionReview
		reviewResponse *v1beta1.AdmissionResponse
	}{
		{
			Name: "Normal Case State Not Set During Creating",
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
					Name:      "normal-case-not-set",
					Operation: "CREATE",
					Object: runtime.RawExtension{
						Raw: stateNotSetJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		},
		{
			Name: "Normal Case Set State of Open During Creating",
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
				Allowed: true,
			},
		},
		{
			Name: "Normal Case Set State of Closed During Creating",
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
					Name:      "normal-case-set-closed",
					Operation: "CREATE",
					Object: runtime.RawExtension{
						Raw: closedStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		},
		{
			Name: "Abnormal Case Wrong State Configured During Creating",
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
					Name:      "abnormal-case",
					Operation: "CREATE",
					Object: runtime.RawExtension{
						Raw: wrongStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: field.Invalid(field.NewPath("requestBody").Child("spec").Child("state"),
						"wrong", fmt.Sprintf("queue state must be in %v", []schedulingv1beta1.QueueState{
							schedulingv1beta1.QueueStateOpen,
							schedulingv1beta1.QueueStateClosed,
						})).Error(),
				},
			},
		},
		{
			Name: "Normal Case Changing State From Open to Closed During Updating",
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
					Name:      "normal-case-open-to-close-updating",
					Operation: "UPDATE",
					OldObject: runtime.RawExtension{
						Raw: openStateJSON,
					},
					Object: runtime.RawExtension{
						Raw: closedStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		},
		{
			Name: "Normal Case Changing State From Closed to Open During Updating",
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
					Name:      "normal-case-closed-to-open-updating",
					Operation: "UPDATE",
					OldObject: runtime.RawExtension{
						Raw: closedStateJSON,
					},
					Object: runtime.RawExtension{
						Raw: openStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		},
		{
			Name: "Abnormal Case Changing State From Open to Wrong State During Updating",
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
					Name:      "abnormal-case-open-to-wrong-state-updating",
					Operation: "UPDATE",
					OldObject: runtime.RawExtension{
						Raw: openStateJSON,
					},
					Object: runtime.RawExtension{
						Raw: wrongStateJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: field.Invalid(field.NewPath("requestBody").Child("spec").Child("state"),
						"wrong", fmt.Sprintf("queue state must be in %v", []schedulingv1beta1.QueueState{
							schedulingv1beta1.QueueStateOpen,
							schedulingv1beta1.QueueStateClosed,
						})).Error(),
				},
			},
		},
		{
			Name: "Normal Case Queue With Closed State Can Be Deleted",
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
					Name:      "closed-state-for-delete",
					Operation: "DELETE",
					Object: runtime.RawExtension{
						Raw: closedStateForDeleteJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: true,
			},
		},
		{
			Name: "Abnormal Case Queue With Open State Can Not Be Deleted",
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
					Name:      "open-state-for-delete",
					Operation: "DELETE",
					Object: runtime.RawExtension{
						Raw: openStateForDeleteJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf("only queue with state `%s` can be deleted, queue `%s` state is `%s`",
						schedulingv1beta1.QueueStateClosed, "open-state-for-delete", schedulingv1beta1.QueueStateOpen),
				},
			},
		},
		{
			Name: "Abnormal Case default Queue Can Not Be Deleted",
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
					Name:      "default",
					Operation: "DELETE",
					Object: runtime.RawExtension{
						Raw: openStateForDeleteJSON,
					},
				},
			},
			reviewResponse: &v1beta1.AdmissionResponse{
				Allowed: false,
				Result: &metav1.Status{
					Message: fmt.Sprintf("`%s` queue can not be deleted", "default"),
				},
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
					Name:      "default",
					Operation: "Invalid",
					Object: runtime.RawExtension{
						Raw: openStateForDeleteJSON,
					},
				},
			},
			reviewResponse: util.ToAdmissionResponse(fmt.Errorf("invalid operation `%s`, "+
				"expect operation to be `CREATE`, `UPDATE` or `DELETE`", "Invalid")),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			reviewResponse := AdmitQueues(testCase.AR)
			if !reflect.DeepEqual(reviewResponse, testCase.reviewResponse) {
				t.Errorf("Test case %s failed, expect %v, got %v", testCase.Name,
					reviewResponse, testCase.reviewResponse)
			}
		})
	}
}
