Soul Mirror 是用来同步多个k8s集群中配置文件的工具。

## 配置

日志级别可以通过loglevel flag来设置。也可以通过:8080/logging?level=debug来配置

### 集群配置

用来配置集群访问凭证。支持设置 kubeconfig 地址和直接写入内容两种配置方式。 两种都配了的时候优先使用文本内容

```yaml
clusters:
  - name: dev
    configPath: ./config/dev
  - name: dev2
    config: -|
    kubeconfig file content...
```

### 任务配置

用于配置需要同步的资源以及相关过滤器。

```yaml
mirrors:
  - name: svc
    config:
      clusters:
        main: dev # 主集群名称
        follower: # 从集群列表。如果不了解filter，最好不要将主集群包含在里面，否则容易导致无限更新资源
          - dev2
      namespace: test # 非必须。设置了则只同步该命名空间的配置
      notInNamespace: test # 非必须。设置了则不同步该命名空间的配置
      syncCreate: true # 非必须，默认为false。是否同步创建事件
      syncDelete: false # 非必须，默认为false。是否同步删除事件
      targetName: demo # 非必须。只同步该名字的资源
    resources: # 待同步资源类型。可以通过kubectl api-resources来查看资源名称，group及版本等信息
      - group: ""
        version: v1
        kind: services # 不要大写，且要求复数形式。最好复制kubectl api-resources中的name
    selector: # 待同步资源过滤器。支持matchLabels和matchExpressions两种过滤方式
      matchLabels:
        foo: bar
    filter: # 同步前预处理待同步资源
      - action: replace
        key: spec.clusterIP
```

### filter

mirror中的filter可以用于修改和删除一些配置

#### replace

如果被同步集群中存在selflink一样的资源，则保留被同步集群中的配置。如果不存在同名资源，则使用默认值。

```yaml
action: replace # 操作名称
key: spec.clusterIP # 配置路径。只允许填写字段名称。无法配置数组index和jsonpath
value: '' # 非必须。可以设置数字，字符串，数组，对象等。在设置字符串时，需要转义双引号，如： '"value"'。
```

当前默认会replace以下字段，默认值为空：

```
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
```

#### set

在待同步资源上为指定字段设置静态值

```yaml
action: set # 操作名称
key: spec.clusterIP # 配置路径。只允许填写字段名称。无法配置数组index和jsonpath
value: '' # 非必须。可以设置数字，字符串，数组，对象等。在设置字符串时，需要转义双引号，如： '"value"'。
```

#### delete

在待同步资源上删除指定字段

```yaml
action: set # 操作名称
key: spec.clusterIP # 配置路径。只允许填写字段名称。无法配置数组index和jsonpath
value: '' # 非必须。可以设置数字，字符串，数组，对象等。在设置字符串时，需要转义双引号，如： '"value"'。
```