package filter

import (
	"soul-mirror/model"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/dynamiclister"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

var (
	mutex      = sync.Mutex{}
	clusterMap = make(map[string]*cluster)
)

type cluster struct {
	name    string
	mirrors map[string]*mirrorController
	cache   map[string]dynamiclister.Lister

	config  *rest.Config
	client  dynamic.Interface
	factory dynamicinformer.DynamicSharedInformerFactory
	// 用于在正式启动服务前拉取缓存
	cacheFactory dynamicinformer.DynamicSharedInformerFactory
}

type mirrorController struct {
	config model.Mirror
	gvr    schema.GroupVersionResource
	client dynamic.Interface
	logger *logrus.Logger

	indexer  cache.Indexer
	informer cache.SharedIndexInformer
	queue    workqueue.RateLimitingInterface
}

func Start(stop chan struct{}) {
	// 缓存
	for _, c := range clusterMap {
		go c.cacheFactory.Start(stop)
	}
	for _, c := range clusterMap {
		c.cacheFactory.WaitForCacheSync(stop)
	}

	// 启动
	for _, c := range clusterMap {
		for _, m := range c.mirrors {
			//go m.informer.Run(stop)
			go m.Run(8, stop)
		}
	}
}

func initCluster(obj *model.Cluster) (err error) {
	_, ok := clusterMap[obj.Name]
	if ok {
		return nil
	}
	c := &cluster{
		name:    obj.Name,
		mirrors: make(map[string]*mirrorController),
		cache:   make(map[string]dynamiclister.Lister),
	}
	err = c.setClient(obj)
	if err != nil {
		return err
	}
	clusterMap[obj.Name] = c
	return
}

func UpdateCluster(obj *model.Cluster) (err error) {
	mutex.Lock()
	defer mutex.Unlock()
	c, ok := clusterMap[obj.Name]
	if !ok {
		err = initCluster(obj)
		if err != nil {
			return
		}
		c, _ = clusterMap[obj.Name]
	}

	err = c.setClient(obj)
	if err != nil {
		return err
	}

	for _, mirror := range c.mirrors {
		c.updateMirror(mirror.config)
	}
	return
}

func (c *cluster) setClient(obj *model.Cluster) (err error) {
	if len(obj.Config) > 0 {
		c.config, err = clientcmd.RESTConfigFromKubeConfig([]byte(obj.Config))
	} else {
		c.config, err = clientcmd.BuildConfigFromFlags("", obj.ConfigPath)
	}
	if err != nil {
		return
	}

	c.client, err = dynamic.NewForConfig(c.config)
	if err != nil {
		return
	}
	c.factory = dynamicinformer.NewDynamicSharedInformerFactory(c.client, 10*time.Minute)
	c.cacheFactory = dynamicinformer.NewDynamicSharedInformerFactory(c.client, 0)
	return
}

func (c *cluster) initMirror(obj model.Mirror) {
	for _, m := range obj.Resources {
		gvr := schema.GroupVersionResource{Group: m.Group, Version: m.Version, Resource: m.Kind}
		mirror := &mirrorController{
			config:   obj,
			gvr:      gvr,
			client:   c.client,
			informer: c.factory.ForResource(gvr).Informer(),
			indexer:  c.factory.ForResource(gvr).Informer().GetIndexer(),
			queue:    workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
			logger:   logrus.WithField("Name", obj.Name).WithField("Main", obj.Name).Logger,
		}
		mirror.informer.AddEventHandler(mirror.genHandler())
		c.mirrors[mirror.String()] = mirror
		for _, cluster := range obj.Config.Clusters.Follower {
			targetCluster := clusterMap[cluster]
			targetCluster.cache[mirror.String()] = dynamiclister.New(targetCluster.cacheFactory.ForResource(gvr).Informer().GetIndexer(), gvr)
		}

		promauto.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "event_queue_length",
			ConstLabels: prometheus.Labels{
				"name": mirror.config.Name,
			},
		}, func() float64 {
			return float64(mirror.queue.Len())
		})
	}
}

func (m *mirrorController) String() string {
	return m.config.Name + m.gvr.String()
}

func UpdateMirror(obj model.Mirror) {
	mutex.Lock()
	defer mutex.Unlock()
	c := clusterMap[obj.Config.Clusters.Main]
	c.updateMirror(obj)
}

func (c *cluster) updateMirror(obj model.Mirror) {
	c.deleteMirror(obj)
	c.initMirror(obj)
}

func (c *cluster) DeleteMirror(obj model.Mirror) {
	mutex.Lock()
	defer mutex.Unlock()
	c.deleteMirror(obj)
}

func (c *cluster) deleteMirror(obj model.Mirror) {
	for _, m := range obj.Resources {
		gvr := schema.GroupVersionResource{Group: m.Group, Version: m.Version, Resource: m.Kind}
		mirror, ok := c.mirrors[obj.Name+gvr.String()]
		if ok {
			mirror.close()
			delete(c.mirrors, mirror.String())
			for _, cluster := range obj.Config.Clusters.Follower {
				delete(clusterMap[cluster].cache, mirror.String())
			}
		}
	}
}

func (m *mirrorController) close() {
	m.queue.ShutDown()
}
