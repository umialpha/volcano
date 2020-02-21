/*
Copyright 2017 The Kubernetes Authors.

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

package allocate

import (
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"volcano.sh/volcano/cmd/scheduler/app/options"
	schedulingv1 "volcano.sh/volcano/pkg/apis/scheduling/v1beta1"
	"volcano.sh/volcano/pkg/scheduler/api"
	"volcano.sh/volcano/pkg/scheduler/cache"
	"volcano.sh/volcano/pkg/scheduler/conf"
	"volcano.sh/volcano/pkg/scheduler/framework"
	"volcano.sh/volcano/pkg/scheduler/plugins/drf"
	"volcano.sh/volcano/pkg/scheduler/plugins/gang"
	"volcano.sh/volcano/pkg/scheduler/plugins/proportion"
	"volcano.sh/volcano/pkg/scheduler/util"
)

func TestAllocate(t *testing.T) {
	framework.RegisterPluginBuilder("drf", drf.New)
	framework.RegisterPluginBuilder("proportion", proportion.New)
	framework.RegisterPluginBuilder("gang", gang.New)

	options.ServerOpts = &options.ServerOption{
		MinNodesToFind:             100,
		MinPercentageOfNodesToFind: 5,
		PercentageOfNodesToFind:    100,
	}

	defer framework.CleanupPluginBuilders()

	tests := []struct {
		name      string
		podGroups []*schedulingv1.PodGroup
		pods      []*v1.Pod
		nodes     []*v1.Node
		queues    []*schedulingv1.Queue
		expected  map[string]string
	}{
		{
			name: "one Job with two Pods on one node",
			podGroups: []*schedulingv1.PodGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg1",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue: "c1",
					},
				},
			},
			pods: []*v1.Pod{
				util.BuildPod("c1", "p1", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p2", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
			},
			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("2", "4Gi"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "c1",
					},
					Spec: schedulingv1.QueueSpec{
						Weight: 1,
					},
				},
			},
			expected: map[string]string{
				"c1/p1": "n1",
				"c1/p2": "n1",
			},
		},
		{
			name: "two Jobs on one node",
			podGroups: []*schedulingv1.PodGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg1",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue: "c1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg2",
						Namespace: "c2",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue: "c2",
					},
				},
			},

			pods: []*v1.Pod{
				// pending pod with owner1, under c1
				util.BuildPod("c1", "p1", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				// pending pod with owner1, under c1
				util.BuildPod("c1", "p2", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				// pending pod with owner2, under c2
				util.BuildPod("c2", "p1", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
				// pending pod with owner, under c2
				util.BuildPod("c2", "p2", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
			},
			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("2", "4G"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "c1",
					},
					Spec: schedulingv1.QueueSpec{
						Weight: 1,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "c2",
					},
					Spec: schedulingv1.QueueSpec{
						Weight: 1,
					},
				},
			},
			expected: map[string]string{
				"c2/p1": "n1",
				"c1/p1": "n1",
			},
		},

		// SubGroup Test 1
		// Three jobs. job1 and job2 belongs to the same subgroup,
		// (job1 || job2) && job3 can be allocated, but job1 && job2 && job3 cannot be allocated,
		// expected: job3 get allocated
		{
			name: "Three jobs, only one job get allocated",
			podGroups: []*schedulingv1.PodGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg1",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "sub1",
						MinMember: 3,
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg2",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "sub1",
						MinMember: 2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg3",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "",
						MinMember: 2,
					},
				},
			},
			pods: []*v1.Pod{
				// p1, p2, p3 in pg1(minMember=3)
				util.BuildPod("c1", "p1", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p2", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p3", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				// p4, p5 in pg2(minMember=2)
				util.BuildPod("c1", "p4", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p5", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
				// p6, p7, p8 in pg3(minMember=2)
				util.BuildPod("c1", "p6", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg3", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p7", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg3", make(map[string]string), make(map[string]string)),
			},

			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("4", "4Gi"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "c1",
					},
					Spec: schedulingv1.QueueSpec{
						Weight: 1,
					},
				},
			},
			expected: map[string]string{
				"c1/p6": "n1",
				"c1/p7": "n1",
			},
		},

		// SubGroup Test 2
		// Three jobs. job1 and job2 belongs to the same subgroup,
		// (job1 && job2) || job3 can be allocated, but job1 && job2 && job3 cannot be allocated,
		// expected: job3 get allocated
		{
			name: "SubGroup Test 2",
			podGroups: []*schedulingv1.PodGroup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg1",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "sub1",
						MinMember: 2,
					},
				},

				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg2",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "sub1",
						MinMember: 2,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pg3",
						Namespace: "c1",
					},
					Spec: schedulingv1.PodGroupSpec{
						Queue:     "c1",
						SubGroup:  "",
						MinMember: 2,
					},
				},
			},
			pods: []*v1.Pod{
				// p1, p2, p3 in pg1(minMember=3)
				util.BuildPod("c1", "p1", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p2", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg1", make(map[string]string), make(map[string]string)),
				// p4, p5 in pg2(minMember=2)
				util.BuildPod("c1", "p4", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p5", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg2", make(map[string]string), make(map[string]string)),
				// p6, p7, p8 in pg3(minMember=2)
				util.BuildPod("c1", "p6", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg3", make(map[string]string), make(map[string]string)),
				util.BuildPod("c1", "p7", "", v1.PodPending, util.BuildResourceList("1", "1G"), "pg3", make(map[string]string), make(map[string]string)),
			},

			nodes: []*v1.Node{
				util.BuildNode("n1", util.BuildResourceList("5", "4Gi"), make(map[string]string)),
			},
			queues: []*schedulingv1.Queue{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "c1",
					},
					Spec: schedulingv1.QueueSpec{
						Weight: 1,
					},
				},
			},
			expected: map[string]string{
				"c1/p1": "n1",
				"c1/p2": "n1",
				"c1/p4": "n1",
				"c1/p5": "n1",
			},
		},
	}

	allocate := New()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			binder := &util.FakeBinder{
				Binds:   map[string]string{},
				Channel: make(chan string),
			}
			schedulerCache := &cache.SchedulerCache{
				Nodes:         make(map[string]*api.NodeInfo),
				Jobs:          make(map[api.JobID]*api.JobInfo),
				Queues:        make(map[api.QueueID]*api.QueueInfo),
				Binder:        binder,
				StatusUpdater: &util.FakeStatusUpdater{},
				VolumeBinder:  &util.FakeVolumeBinder{},

				Recorder: record.NewFakeRecorder(100),
			}
			for _, node := range test.nodes {
				schedulerCache.AddNode(node)
			}
			for _, pod := range test.pods {
				schedulerCache.AddPod(pod)
			}

			for _, ss := range test.podGroups {
				schedulerCache.AddPodGroupV1beta1(ss)
			}

			for _, q := range test.queues {
				schedulerCache.AddQueueV1beta1(q)
			}

			trueValue := true
			ssn := framework.OpenSession(schedulerCache, []conf.Tier{
				{
					Plugins: []conf.PluginOption{
						{
							Name:                  "drf",
							EnabledPreemptable:    &trueValue,
							EnabledJobOrder:       &trueValue,
							EnabledNamespaceOrder: &trueValue,
						},
						{
							Name:               "proportion",
							EnabledQueueOrder:  &trueValue,
							EnabledReclaimable: &trueValue,
						},
						{
							Name:            "gang",
							EnabledJobReady: &trueValue,
						},
					},
				},
			}, nil)
			defer framework.CloseSession(ssn)

			allocate.Execute(ssn)

			for i := 0; i < len(test.expected); i++ {
				select {
				case <-binder.Channel:
				case <-time.After(3 * time.Second):
					t.Errorf("Failed to get binding request.")
				}
			}

			if !reflect.DeepEqual(test.expected, binder.Binds) {
				t.Errorf("expected: %v, got %v ", test.expected, binder.Binds)
			}
		})
	}
}
