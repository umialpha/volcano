/*
Copyright 2017 The Volcano Authors.

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

package job

import (
	"fmt"
	"hash"
	"hash/fnv"
	"k8s.io/api/core/v1"
	"k8s.io/api/scheduling/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	kubeschedulinginformers "k8s.io/client-go/informers/scheduling/v1beta1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	corelisters "k8s.io/client-go/listers/core/v1"
	kubeschedulinglisters "k8s.io/client-go/listers/scheduling/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sync"
	"time"

	batchv1alpha1 "volcano.sh/volcano/pkg/apis/batch/v1alpha1"
	busv1alpha1 "volcano.sh/volcano/pkg/apis/bus/v1alpha1"
	vcclientset "volcano.sh/volcano/pkg/client/clientset/versioned"
	vcscheme "volcano.sh/volcano/pkg/client/clientset/versioned/scheme"
	informerfactory "volcano.sh/volcano/pkg/client/informers/externalversions"
	batchinformer "volcano.sh/volcano/pkg/client/informers/externalversions/batch/v1alpha1"
	businformer "volcano.sh/volcano/pkg/client/informers/externalversions/bus/v1alpha1"
	schedulinginformers "volcano.sh/volcano/pkg/client/informers/externalversions/scheduling/v1beta1"
	batchlister "volcano.sh/volcano/pkg/client/listers/batch/v1alpha1"
	buslister "volcano.sh/volcano/pkg/client/listers/bus/v1alpha1"
	schedulinglisters "volcano.sh/volcano/pkg/client/listers/scheduling/v1beta1"
	"volcano.sh/volcano/pkg/controllers/apis"
	jobcache "volcano.sh/volcano/pkg/controllers/cache"
	"volcano.sh/volcano/pkg/controllers/job/state"
)

const (
	// maxRetries is the number of times a volcano job will be retried before it is dropped out of the queue.
	// With the current rate-limiter in use (5ms*2^(maxRetries-1)) the following numbers represent the times
	// a volcano job is going to be requeued:
	//
	// 5ms, 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1.3s, 2.6s, 5.1s, 10.2s, 20.4s, 41s, 82s
	maxRetries = 15
)

// Controller the Job Controller type
type Controller struct {
	kubeClient kubernetes.Interface
	vcClient   vcclientset.Interface

	jobInformer batchinformer.JobInformer
	podInformer coreinformers.PodInformer
	pvcInformer coreinformers.PersistentVolumeClaimInformer
	pgInformer  schedulinginformers.PodGroupInformer
	svcInformer coreinformers.ServiceInformer
	cmdInformer businformer.CommandInformer
	pcInformer  kubeschedulinginformers.PriorityClassInformer

	// A store of jobs
	jobLister batchlister.JobLister
	jobSynced func() bool

	// A store of pods
	podLister corelisters.PodLister
	podSynced func() bool

	pvcLister corelisters.PersistentVolumeClaimLister
	pvcSynced func() bool

	// A store of podgroups
	pgLister schedulinglisters.PodGroupLister
	pgSynced func() bool

	// A store of service
	svcLister corelisters.ServiceLister
	svcSynced func() bool

	cmdLister buslister.CommandLister
	cmdSynced func() bool

	pcLister kubeschedulinglisters.PriorityClassLister
	pcSynced func() bool

	// queue that need to sync up
	queueList    []workqueue.RateLimitingInterface
	commandQueue workqueue.RateLimitingInterface
	cache        jobcache.Cache
	//Job Event recorder
	recorder        record.EventRecorder
	priorityClasses map[string]*v1beta1.PriorityClass

	sync.Mutex
	errTasks workqueue.RateLimitingInterface
	workers  uint32
}

// NewJobController create new Job Controller
func NewJobController(
	kubeClient kubernetes.Interface,
	vcClient vcclientset.Interface,
	sharedInformers informers.SharedInformerFactory,
	workers uint32,
) *Controller {

	//Initialize event client
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(vcscheme.Scheme, v1.EventSource{Component: "vc-controller-manager"})

	cc := &Controller{
		kubeClient:      kubeClient,
		vcClient:        vcClient,
		queueList:       make([]workqueue.RateLimitingInterface, workers, workers),
		commandQueue:    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
		cache:           jobcache.New(),
		errTasks:        newRateLimitingQueue(),
		recorder:        recorder,
		priorityClasses: make(map[string]*v1beta1.PriorityClass),
		workers:         workers,
	}
	var i uint32
	for i = 0; i < workers; i++ {
		cc.queueList[i] = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	}

	cc.jobInformer = informerfactory.NewSharedInformerFactory(cc.vcClient, 0).Batch().V1alpha1().Jobs()
	cc.jobInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: cc.addJob,
		// TODO: enable this until we find an appropriate way.
		UpdateFunc: cc.updateJob,
		DeleteFunc: cc.deleteJob,
	})
	cc.jobLister = cc.jobInformer.Lister()
	cc.jobSynced = cc.jobInformer.Informer().HasSynced

	cc.cmdInformer = informerfactory.NewSharedInformerFactory(cc.vcClient, 0).Bus().V1alpha1().Commands()
	cc.cmdInformer.Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch obj.(type) {
				case *busv1alpha1.Command:
					cmd := obj.(*busv1alpha1.Command)
					if cmd.TargetObject != nil &&
						cmd.TargetObject.APIVersion == batchv1alpha1.SchemeGroupVersion.String() &&
						cmd.TargetObject.Kind == "Job" {
						return true
					}

					return false
				default:
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: cc.addCommand,
			},
		},
	)
	cc.cmdLister = cc.cmdInformer.Lister()
	cc.cmdSynced = cc.cmdInformer.Informer().HasSynced

	cc.podInformer = sharedInformers.Core().V1().Pods()
	cc.podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cc.addPod,
		UpdateFunc: cc.updatePod,
		DeleteFunc: cc.deletePod,
	})

	cc.podLister = cc.podInformer.Lister()
	cc.podSynced = cc.podInformer.Informer().HasSynced

	cc.pvcInformer = sharedInformers.Core().V1().PersistentVolumeClaims()
	cc.pvcLister = cc.pvcInformer.Lister()
	cc.pvcSynced = cc.pvcInformer.Informer().HasSynced

	cc.svcInformer = sharedInformers.Core().V1().Services()
	cc.svcLister = cc.svcInformer.Lister()
	cc.svcSynced = cc.svcInformer.Informer().HasSynced

	cc.pgInformer = informerfactory.NewSharedInformerFactory(cc.vcClient, 0).Scheduling().V1beta1().PodGroups()
	cc.pgInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		UpdateFunc: cc.updatePodGroup,
	})
	cc.pgLister = cc.pgInformer.Lister()
	cc.pgSynced = cc.pgInformer.Informer().HasSynced

	cc.pcInformer = sharedInformers.Scheduling().V1beta1().PriorityClasses()
	cc.pcInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    cc.addPriorityClass,
		DeleteFunc: cc.deletePriorityClass,
	})
	cc.pcLister = cc.pcInformer.Lister()
	cc.pcSynced = cc.pcInformer.Informer().HasSynced

	// Register actions
	state.SyncJob = cc.syncJob
	state.KillJob = cc.killJob

	return cc
}

// Run start JobController
func (cc *Controller) Run(stopCh <-chan struct{}) {

	go cc.jobInformer.Informer().Run(stopCh)
	go cc.podInformer.Informer().Run(stopCh)
	go cc.pvcInformer.Informer().Run(stopCh)
	go cc.pgInformer.Informer().Run(stopCh)
	go cc.svcInformer.Informer().Run(stopCh)
	go cc.cmdInformer.Informer().Run(stopCh)
	go cc.pcInformer.Informer().Run(stopCh)

	cache.WaitForCacheSync(stopCh, cc.jobSynced, cc.podSynced, cc.pgSynced,
		cc.svcSynced, cc.cmdSynced, cc.pvcSynced, cc.pcSynced)

	go wait.Until(cc.handleCommands, 0, stopCh)
	var i uint32
	for i = 0; i < cc.workers; i++ {
		go func(num uint32) {
			wait.Until(
				func() {
					cc.worker(num)
				},
				time.Second,
				stopCh)
		}(i)
	}

	go cc.cache.Run(stopCh)

	// Re-sync error tasks.
	go wait.Until(cc.processResyncTask, 0, stopCh)

	klog.Infof("JobController is running ...... ")
}

func (cc *Controller) worker(i uint32) {
	klog.Infof("worker %d start ...... ", i)

	for cc.processNextReq(i) {
	}
}

func (cc *Controller) belongsToThisRoutine(key string, count uint32) bool {
	var hashVal hash.Hash32
	var val uint32

	hashVal = fnv.New32()
	hashVal.Write([]byte(key))

	val = hashVal.Sum32()

	if val%cc.workers == count {
		return true
	}

	return false
}

func (cc *Controller) getWorkerQueue(key string) workqueue.RateLimitingInterface {
	var hashVal hash.Hash32
	var val uint32

	hashVal = fnv.New32()
	hashVal.Write([]byte(key))

	val = hashVal.Sum32()

	queue := cc.queueList[val%cc.workers]

	return queue
}

func (cc *Controller) processNextReq(count uint32) bool {
	queue := cc.queueList[count]
	obj, shutdown := queue.Get()
	if shutdown {
		klog.Errorf("Fail to pop item from queue")
		return false
	}

	req := obj.(apis.Request)
	defer queue.Done(req)

	key := jobcache.JobKeyByReq(&req)
	if !cc.belongsToThisRoutine(key, count) {
		klog.Errorf("should not occur The job does not belongs to this routine key:%s, worker:%d...... ", key, count)
		queueLocal := cc.getWorkerQueue(key)
		queueLocal.Add(req)
		return true
	}

	klog.V(3).Infof("Try to handle request <%v>", req)

	jobInfo, err := cc.cache.Get(jobcache.JobKeyByReq(&req))
	if err != nil {
		// TODO(k82cn): ignore not-ready error.
		klog.Errorf("Failed to get job by <%v> from cache: %v", req, err)
		return true
	}

	st := state.NewState(jobInfo)
	if st == nil {
		klog.Errorf("Invalid state <%s> of Job <%v/%v>",
			jobInfo.Job.Status.State, jobInfo.Job.Namespace, jobInfo.Job.Name)
		return true
	}

	action := applyPolicies(jobInfo.Job, &req)
	klog.V(3).Infof("Execute <%v> on Job <%s/%s> in <%s> by <%T>.",
		action, req.Namespace, req.JobName, jobInfo.Job.Status.State.Phase, st)

	if action != busv1alpha1.SyncJobAction {
		cc.recordJobEvent(jobInfo.Job.Namespace, jobInfo.Job.Name, batchv1alpha1.ExecuteAction, fmt.Sprintf(
			"Start to execute action %s ", action))
	}

	if err := st.Execute(action); err != nil {
		if queue.NumRequeues(req) < maxRetries {
			klog.V(2).Infof("Failed to handle Job <%s/%s>: %v",
				jobInfo.Job.Namespace, jobInfo.Job.Name, err)
			// If any error, requeue it.
			queue.AddRateLimited(req)
			return true
		}
		cc.recordJobEvent(jobInfo.Job.Namespace, jobInfo.Job.Name, batchv1alpha1.ExecuteAction, fmt.Sprintf(
			"Job failed on action %s for retry limit reached", action))
		klog.Warningf("Dropping job<%s/%s> out of the queue: %v because max retries has reached", jobInfo.Job.Namespace, jobInfo.Job.Name, err)
	}

	// If no error, forget it.
	queue.Forget(req)

	return true
}
