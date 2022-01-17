package filter

import (
	"context"
	"encoding/json"
	"soul-mirror/model"
	"strconv"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamiclister"
	"k8s.io/client-go/tools/cache"
)

func (m *mirrorController) genHandler() cache.ResourceEventHandler {
	selector, _ := metav1.LabelSelectorAsSelector(m.config.Selector)
	if m.config.Selector == nil {
		selector = labels.Everything()
	}
	match := func(object *unstructured.Unstructured) bool {
		if len(m.config.Config.Namespace) > 0 && object.GetNamespace() != m.config.Config.Namespace {
			return false
		}
		if len(m.config.Config.NotInNamespace) > 0 && object.GetNamespace() == m.config.Config.NotInNamespace {
			return false
		}
		if !selector.Matches(labels.Set(object.GetLabels())) {
			return false
		}
		if len(m.config.Config.TargetName) > 0 && object.GetName() != m.config.Config.TargetName {
			return false
		}
		return true
	}

	return cache.ResourceEventHandlerFuncs{AddFunc: func(obj interface{}) {
		if !m.config.Config.SyncCreate {
			return
		}
		object := obj.(*unstructured.Unstructured)
		if !match(object) {
			return
		}
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err == nil {
			m.queue.Add(key)
		}
	}, UpdateFunc: func(oldObj, newObj interface{}) {
		obj := newObj.(*unstructured.Unstructured)
		if !match(obj) {
			return
		}
		key, err := cache.MetaNamespaceKeyFunc(obj)
		if err == nil {
			m.queue.Add(key)
		}
	}, DeleteFunc: func(obj interface{}) {
		if !m.config.Config.SyncDelete {
			return
		}
		object := obj.(*unstructured.Unstructured)
		if !match(object) {
			return
		}
		key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
		if err == nil {
			m.queue.Add(key)
		}
	}}
}

func (m *mirrorController) delete(cluster *cluster, key string) error {
	client := m.getTargetClientFromKey(cluster, key)
	_, name, _ := cache.SplitMetaNamespaceKey(key)
	err := client.Delete(context.TODO(), name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return nil
	} else if err != nil {
		m.logger.WithField("to", cluster.name).WithError(err).Errorf("failed to delete %s", key)
		EventHandleErrorCount.WithLabelValues(m.config.Name, "delete", string(errors.ReasonForError(err))).Inc()
		return err
	}
	EventHandleCount.WithLabelValues(m.config.Name, "deleted").Inc()
	return nil
}

func (m *mirrorController) add(cluster *cluster, srcJson []byte, srcObject *unstructured.Unstructured) error {
	client := m.getTargetClient(cluster, srcObject)
	res := m.filter(srcJson, []byte{})
	resObject := &unstructured.Unstructured{}
	_ = json.Unmarshal(res, resObject)

	annotation := resObject.GetAnnotations()
	if annotation == nil {
		annotation = make(map[string]string)
	}
	annotation[model.ResourceVersionAnnotation] = srcObject.GetResourceVersion()
	resObject.SetAnnotations(annotation)

	_, err := client.Create(context.TODO(), resObject, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		m.logger.WithField("to", cluster.name).WithError(err).Errorf("failed to create %s", m.fmtMeta(resObject))
		EventHandleErrorCount.WithLabelValues(m.config.Name, "add", string(errors.ReasonForError(err))).Inc()
		return err
	}
	EventHandleCount.WithLabelValues(m.config.Name, "added").Inc()
	return nil
}

func (m *mirrorController) update(cluster *cluster, srcJson []byte, srcObject *unstructured.Unstructured) error {
	client := m.getTargetClient(cluster, srcObject)
	targetObject, err := m.getTargetLister(cluster).Get(m.fmtMeta(srcObject))
	if errors.IsNotFound(err) {
		return m.add(cluster, srcJson, srcObject)
	} else if err != nil {
		m.logger.WithField("to", cluster.name).WithError(err).Errorf("failed to get %s", m.fmtMeta(srcObject))
		EventHandleErrorCount.WithLabelValues(m.config.Name, "update", string(errors.ReasonForError(err))).Inc()
		return err
	}

	annotation := targetObject.GetAnnotations()
	if annotation == nil {
		annotation = make(map[string]string)
	}
	targetResVer, _ := strconv.ParseInt(annotation[model.ResourceVersionAnnotation], 10, 64)
	srcResVer, _ := strconv.ParseInt(srcObject.GetResourceVersion(), 10, 64)
	if targetResVer >= srcResVer {
		m.logger.WithField("to", cluster.name).Infof("synced version %v %s", srcObject.GetResourceVersion(), m.fmtMeta(srcObject))
		EventHandleCount.WithLabelValues(m.config.Name, "synced").Inc()
		return nil
	}
	annotation[model.ResourceVersionAnnotation] = strconv.FormatInt(srcResVer, 10)
	targetObject.SetAnnotations(annotation)

	target, _ := json.Marshal(targetObject)
	res := m.filter(srcJson, target)
	resObject := &unstructured.Unstructured{}
	err = json.Unmarshal(res, resObject)
	_, err = client.Update(context.TODO(), resObject, metav1.UpdateOptions{})
	if err != nil && errors.IsConflict(err) {
		m.logger.WithField("to", cluster.name).Debugf("failed to update %s : conflict", m.fmtMeta(resObject))
		EventHandleCount.WithLabelValues(m.config.Name, "conflict").Inc()
		return err
	}
	if err != nil {
		m.logger.WithField("to", cluster.name).WithError(err).Errorf("failed to update %s", m.fmtMeta(resObject))
		m.logger.WithField("to", cluster.name).Debugf("failed to update %s : %s", m.fmtMeta(resObject), res)
		EventHandleErrorCount.WithLabelValues(m.config.Name, "update", string(errors.ReasonForError(err))).Inc()
		return err
	}
	EventHandleCount.WithLabelValues(m.config.Name, "update").Inc()
	return nil
}

func (m *mirrorController) getTargetClient(cluster *cluster, object *unstructured.Unstructured) dynamic.ResourceInterface {
	if len(object.GetNamespace()) != 0 {
		return cluster.client.Resource(m.gvr).Namespace(object.GetNamespace())
	}
	return cluster.client.Resource(m.gvr)
}

func (m *mirrorController) getTargetClientFromKey(cluster *cluster, key string) dynamic.ResourceInterface {
	ns, _, _ := cache.SplitMetaNamespaceKey(key)
	if len(ns) != 0 {
		return cluster.client.Resource(m.gvr).Namespace(ns)
	}
	return cluster.client.Resource(m.gvr)
}

func (m *mirrorController) getTargetLister(cluster *cluster) dynamiclister.Lister {
	return cluster.cache[m.String()]
}

func (m *mirrorController) fmtMeta(obj *unstructured.Unstructured) string {
	if len(obj.GetNamespace()) > 0 {
		return obj.GetNamespace() + "/" + obj.GetName()
	}
	return obj.GetName()
}
