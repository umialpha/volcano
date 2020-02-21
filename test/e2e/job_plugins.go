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

package e2e

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	cv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/api"

	"volcano.sh/volcano/pkg/controllers/job/helpers"
	"volcano.sh/volcano/pkg/controllers/job/plugins/env"
	"volcano.sh/volcano/pkg/controllers/job/plugins/svc"
)

var _ = Describe("Job E2E Test: Test Job Plugins", func() {
	It("SVC Plugin with Node Affinity", func() {
		jobName := "job-with-svc-plugin"
		namespace := "test"
		taskName := "task"
		foundVolume := false
		context := initTestContext(options{})
		defer cleanupTestContext(context)

		nodeName, rep := computeNode(context, oneCPU)
		Expect(rep).NotTo(Equal(0))

		affinity := &cv1.Affinity{
			NodeAffinity: &cv1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &cv1.NodeSelector{
					NodeSelectorTerms: []cv1.NodeSelectorTerm{
						{
							MatchFields: []cv1.NodeSelectorRequirement{
								{
									Key:      api.NodeFieldSelectorKeyNodeName,
									Operator: cv1.NodeSelectorOpIn,
									Values:   []string{nodeName},
								},
							},
						},
					},
				},
			},
		}

		job := createJob(context, &jobSpec{
			namespace: namespace,
			name:      jobName,
			plugins: map[string][]string{
				"svc": {},
			},
			tasks: []taskSpec{
				{
					img:      defaultNginxImage,
					req:      oneCPU,
					min:      1,
					rep:      1,
					name:     taskName,
					affinity: affinity,
				},
			},
		})

		err := waitJobReady(context, job)
		Expect(err).NotTo(HaveOccurred())

		pluginName := fmt.Sprintf("%s-svc", jobName)
		_, err = context.kubeclient.CoreV1().ConfigMaps(namespace).Get(
			pluginName, v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		pod, err := context.kubeclient.CoreV1().Pods(namespace).Get(
			fmt.Sprintf(helpers.PodNameFmt, jobName, taskName, 0), v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == pluginName {
				foundVolume = true
				break
			}
		}
		Expect(foundVolume).To(BeTrue())

		pods := getTasksOfJob(context, job)
		for _, pod := range pods {
			Expect(pod.Spec.NodeName).To(Equal(nodeName))
		}
	})

	It("SSh Plugin with Pod Affinity", func() {
		jobName := "job-with-ssh-plugin"
		namespace := "test"
		taskName := "task"
		foundVolume := false
		context := initTestContext(options{})
		defer cleanupTestContext(context)

		_, rep := computeNode(context, oneCPU)
		Expect(rep).NotTo(Equal(0))

		labels := map[string]string{"foo": "bar"}

		affinity := &cv1.Affinity{
			PodAffinity: &cv1.PodAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []cv1.PodAffinityTerm{
					{
						LabelSelector: &v1.LabelSelector{
							MatchLabels: labels,
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		}

		job := createJob(context, &jobSpec{
			namespace: namespace,
			name:      jobName,
			plugins: map[string][]string{
				"ssh": {"--no-root"},
			},
			tasks: []taskSpec{
				{
					img:      defaultNginxImage,
					req:      oneCPU,
					min:      rep,
					rep:      rep,
					affinity: affinity,
					labels:   labels,
					name:     taskName,
				},
			},
		})

		err := waitJobReady(context, job)
		Expect(err).NotTo(HaveOccurred())

		pluginName := fmt.Sprintf("%s-%s-ssh", jobName, job.UID)
		_, err = context.kubeclient.CoreV1().Secrets(namespace).Get(
			pluginName, v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		pod, err := context.kubeclient.CoreV1().Pods(namespace).Get(
			fmt.Sprintf(helpers.PodNameFmt, jobName, taskName, 0), v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == pluginName {
				foundVolume = true
				break
			}
		}
		Expect(foundVolume).To(BeTrue())

		pods := getTasksOfJob(context, job)
		// All pods should be scheduled to the same node.
		nodeName := pods[0].Spec.NodeName
		for _, pod := range pods {
			Expect(pod.Spec.NodeName).To(Equal(nodeName))
		}
	})

	It("Check Functionality of all plugins", func() {
		jobName := "job-with-all-plugin"
		namespace := "test"
		taskName := "task"
		foundVolume := false
		foundEnv := false
		context := initTestContext(options{})
		defer cleanupTestContext(context)

		_, rep := computeNode(context, oneCPU)
		Expect(rep).NotTo(Equal(0))

		job := createJob(context, &jobSpec{
			namespace: namespace,
			name:      jobName,
			plugins: map[string][]string{
				"ssh": {"--no-root"},
				"env": {},
				"svc": {},
			},
			tasks: []taskSpec{
				{
					img:  defaultNginxImage,
					req:  oneCPU,
					min:  1,
					rep:  rep,
					name: taskName,
				},
			},
		})

		err := waitJobReady(context, job)
		Expect(err).NotTo(HaveOccurred())

		pluginName := fmt.Sprintf("%s-%s-ssh", jobName, job.UID)
		_, err = context.kubeclient.CoreV1().Secrets(namespace).Get(
			pluginName, v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		pod, err := context.kubeclient.CoreV1().Pods(namespace).Get(
			fmt.Sprintf(helpers.PodNameFmt, jobName, taskName, 0), v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, volume := range pod.Spec.Volumes {
			if volume.Name == pluginName {
				foundVolume = true
				break
			}
		}
		Expect(foundVolume).To(BeTrue())

		// Check whether env exists in the containers and initContainers
		containers := pod.Spec.Containers
		containers = append(containers, pod.Spec.InitContainers...)
		envNames := []string{
			env.TaskVkIndex,
			env.TaskIndex,
			fmt.Sprintf(svc.EnvTaskHostFmt, strings.ToUpper(taskName)),
			fmt.Sprintf(svc.EnvHostNumFmt, strings.ToUpper(taskName)),
		}

		for _, container := range containers {
			for _, name := range envNames {
				foundEnv = false
				for _, envi := range container.Env {
					if envi.Name == name {
						foundEnv = true
						break
					}
				}

				Expect(foundEnv).To(BeTrue(),
					fmt.Sprintf("container: %s, env name: %s", container.Name, name))
			}
		}

		// Check whether service is created with job name
		_, err = context.kubeclient.CoreV1().Services(job.Namespace).Get(job.Name, v1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})
})
