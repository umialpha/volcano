/*
Copyright 2019 The Volcano Authors.

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

package util

import "k8s.io/api/admission/v1beta1"
import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
import "k8s.io/klog"

//ToAdmissionResponse updates the admission response with the input error
func ToAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	klog.Error(err)
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}
