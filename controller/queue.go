package filter

import (
	"encoding/json"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

func (m *mirrorController) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := m.queue.Get()
	if quit {
		return false
	}
	defer m.queue.Done(key)

	err := m.sync(key.(string))
	m.handleErr(err, key)
	return true
}

func (m *mirrorController) sync(key string) error {
	o, exists, err := m.indexer.GetByKey(key)
	if err != nil {
		m.logger.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}
	startTime := time.Now()

	// 删除事件
	if !exists {
		m.logger.Debugf("deleting %s %s", m.config.Name, key)
		defer EventHandleDuration.WithLabelValues(m.config.Name, "delete").Observe(float64(time.Since(startTime).Milliseconds()))
		for _, clusterName := range m.config.Config.Clusters.Follower {
			cluster, ok := clusterMap[clusterName]
			if !ok {
				continue
			}
			err = m.delete(cluster, key)
			if err == nil {
				m.logger.WithField("follower", clusterName).Debugf("deleted %s %s", m.config.Name, key)
			} else {
				return err
			}
		}
		return err
	}

	// 更新事件
	obj := o.(*unstructured.Unstructured)
	m.logger.Debugf("updating %s", key)
	defer EventHandleDuration.WithLabelValues(m.config.Name, "update").Observe(float64(time.Since(startTime).Microseconds()) / 1000)
	src, _ := json.Marshal(obj)
	for _, clusterName := range m.config.Config.Clusters.Follower {
		cluster, ok := clusterMap[clusterName]
		if !ok {
			continue
		}
		err = m.update(cluster, src, obj)
		if err == nil {
			m.logger.WithField("follower", clusterName).Debugf("updated %s", key)
		} else {
			return err
		}
	}
	return nil
}

func (m *mirrorController) handleErr(err error, key interface{}) {
	if err == nil {
		m.queue.Forget(key)
		return
	}

	m.logger.Infof("Error syncing %v: %v", key, err)
	m.queue.AddRateLimited(key)
	EventHandleRetryCount.WithLabelValues(m.config.Name).Inc()
	return
}

// Run begins watching and syncing.
func (m *mirrorController) Run(workers int, stopCh chan struct{}) {
	defer runtime.HandleCrash()
	defer m.queue.ShutDown()

	go m.informer.Run(stopCh)

	if !cache.WaitForCacheSync(stopCh, m.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < workers; i++ {
		go wait.Until(m.runWorker, time.Second, stopCh)
	}

	<-stopCh
}

func (m *mirrorController) runWorker() {
	for m.processNextItem() {
	}
}
