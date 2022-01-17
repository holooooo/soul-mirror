package filter

import (
	"strings"

	"github.com/buger/jsonparser"
	"github.com/sirupsen/logrus"
)

var (
	defaultIgnore = []string{
		"metadata.annotations",
		"metadata.creationTimestamp",
		"metadata.deletionGracePeriodSeconds",
		"metadata.deletionTimestamp",
		"metadata.finalizers",
		"metadata.generateName",
		"metadata.generation",
		"metadata.managedFields",
		"metadata.ownerReferences",
		"metadata.resourceVersion",
		"metadata.selfLink",
		"metadata.uid",
		"status",
		"secrets",
	}
)

func (m *mirrorController) filter(src, target []byte) []byte {
	for _, key := range defaultIgnore {
		src = replace(key, []byte{}, src, target)
	}

	for _, filter := range m.config.Filter {
		switch filter.Action {
		case "replace":
			src = replace(filter.Key, []byte(filter.Value), src, target)
		case "delete":
			src = replace(filter.Key, []byte{}, src, []byte{})
		case "set":
			src = replace(filter.Key, []byte(filter.Value), src, []byte{})
		default:
			logrus.Warnf("Unexpected filter action on %v: %v", m.config.Name, filter.Action)
		}
	}
	return src
}

func replace(key string, defaultValue, src, target []byte) []byte {
	path := strings.Split(key, ".")
	v, datatype, offset, err := jsonparser.Get(target, path...)
	if err == jsonparser.KeyPathNotFoundError {
		v = defaultValue
	} else if datatype == jsonparser.String {
		v = target[offset-len(v)-2 : offset]
	} else if err != nil {
		return src
	}

	var res []byte
	if len(v) == 0 {
		res = jsonparser.Delete(src, path...)
	} else {
		res, err = jsonparser.Set(src, v, path...)
	}
	if len(res) > 0 {
		return res
	}
	logrus.Warning("key")
	return src
}
