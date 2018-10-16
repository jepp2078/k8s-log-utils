package main

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
	appsV1 "k8s.io/api/apps/v1"
	coreV1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

const(
	LogUtilAnnotationName = "dk.coop.integration/log-level"
	LogUtilEnvName = "LOGGING_CONFIG"
	LogUtilEnvDebugValue = "http://configuration-server:9999/default/default/master/logging/log4j2-all.xml"
	LogUtilEnvInfoValue = "http://configuration-server:9999/default/default/master/logging/log4j2-normal.xml"
	LogUtilEnvNoneValue = "http://configuration-server:9999/default/default/master/logging/log4j2-none.xml"


)

var logUtilEnvVarSet = map[string]string{
	"info": LogUtilEnvInfoValue,
	"debug": LogUtilEnvDebugValue,
	"none": LogUtilEnvNoneValue,
}

var logLevelEnvVar = &coreV1.EnvVar{
	Name:    LogUtilEnvName,
}

// Controller struct defines how a controller should encapsulate
// logging, client connectivity, informing (list and watching)
// queueing, and handling of resource changes
type Controller struct {
	logger    *log.Entry
	clientset kubernetes.Interface
	queue     workqueue.RateLimitingInterface
	informer  cache.SharedIndexInformer
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(stopCh <-chan struct{}) {

	// handle a panic with logging and exiting
	defer utilruntime.HandleCrash()
	// ignore new items in the queue but when all goroutines
	// have completed existing items then shutdown
	defer c.queue.ShutDown()

	// run the informer to start listing and watching resources
	go c.informer.Run(stopCh)

	// do the initial synchronization (one time) to populate resources
	if !cache.WaitForCacheSync(stopCh, c.HasSynced) {
		utilruntime.HandleError(fmt.Errorf("error syncing cache"))
		return
	}

	// run the runWorker method every second with a stop channel
	wait.Until(c.runWorker, time.Second, stopCh)
}

// HasSynced allows us to satisfy the Controller interface
// by wiring up the informer's HasSynced method to it
func (c *Controller) HasSynced() bool {
	return c.informer.HasSynced()
}

// runWorker executes the loop to process new items added to the queue
func (c *Controller) runWorker() {
	// invoke processNextItem to fetch and consume the next change
	// to a watched or listed resource
	for c.processNextItem() {
	}
}

// processNextItem retrieves each queued item and takes the
// necessary handler action based off of if the item was
// created or deleted
func (c *Controller) processNextItem() bool {
	// fetch the next item (blocking) from the queue to process or
	// if a shutdown is requested then return out of this to stop
	// processing
	key, quit := c.queue.Get()

	// stop the worker loop from running as this indicates we
	// have sent a shutdown message that the queue has indicated
	// from the Get method
	if quit {
		return false
	}

	defer c.queue.Done(key)

	// assert the string out of the key (format `namespace/name`)
	keyRaw := key.(string)

	// take the string key and get the object out of the indexer
	//
	// item will contain the complex object for the resource and
	// exists is a bool that'll indicate whether or not the
	// resource was created (true) or deleted (false)
	//
	// if there is an error in getting the key from the index
	// then we want to retry this particular queue key a certain
	// number of times (5 here) before we forget the queue key
	// and throw an error
	item, exists, err := c.informer.GetIndexer().GetByKey(keyRaw)
	if err != nil {
		if c.queue.NumRequeues(key) < 5 {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, retrying", key, err)
			c.queue.AddRateLimited(key)
		} else {
			c.logger.Errorf("Controller.processNextItem: Failed processing item with key %s with error %v, no more retries", key, err)
			c.queue.Forget(key)
			utilruntime.HandleError(err)
		}
	}

	// if the item doesn't exist then it was deleted and we need to fire off the handler's
	// ObjectDeleted method. but if the object does exist that indicates that the object
	// was created (or updated) so run the ObjectCreated method
	//
	// after both instances, we want to forget the key from the queue, as this indicates
	// a code path of successful queue key processing

	if !exists {
		//Deployment deleted. Do nothing, annotation is gone with the deployment.
	} else {
		deployment := item.(*appsV1.Deployment)
		c.logger.Printf("Deployment discovered: %s", deployment.Name)
		annotations := deployment.Annotations
		if _, found := annotations[LogUtilAnnotationName]; found {
			c.logger.Printf("Annotation found")
			err := c.addEnvVariableToDeployment(deployment, annotations[LogUtilAnnotationName])
			if err != nil {
				if c.queue.NumRequeues(key) < 3 {
					c.logger.Error(err)
					c.logger.Errorf("Re-queuing key %v more time",(c.queue.NumRequeues(key)-3)*-1)
					c.queue.AddRateLimited(key)
				}else{
					c.queue.Forget(key)
					c.logger.Errorf("Could't add EnvVar to containers. Forgetting deployment: %s", deployment.Name)
				}
			}
		}else{
			c.logger.Printf("Skipping. Annotation not found.")
		}
	}

	// keep the worker loop running by returning true
	return true
}

func (c *Controller) addEnvVariableToDeployment(obj *appsV1.Deployment, logLevel string) error {
	var envVarValue, found = logUtilEnvVarSet[logLevel]

	if found {
		logLevelEnvVar.Value = envVarValue
	}else{
		return fmt.Errorf("LogLevel '%s' not supported", logLevel)
	}

	deployment := obj.DeepCopy()

	for i,container := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[i].Env = append(container.Env, *logLevelEnvVar)
	}

	c.logger.Printf("Setting LogLevel to %s", logLevel)
	_, err := c.clientset.AppsV1().Deployments("default").Update(deployment)

	if err != nil {
		return fmt.Errorf("%s", err)
	}
	return nil
}