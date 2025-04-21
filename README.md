# 使用kubebuilder实现自定义controller

### 参考项目

https://github.com/baidingtech/operator-lesson-demo/blob/main/kubebuilder-demo

### 需求

### ![](img\img.png)

```shell
kubebuilder create api --group ingress --version v1beta1 --kind App
```

### 修改app_types.go

首先，我们需要定义好自定义的资源，我们这里指定为App，我们希望开发团队能够声明一个App
的资源，然后由我们的自定义controller根据其配置，自动为其创建deployment、service、
ingress等资源。

定义如下：

```yaml
type AppSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	EnableSvc     *bool  `json:"enableSvc,omitempty"`
	EnableIngress *bool  `json:"enableIngress,omitempty"`
	Replicas      *int32 `json:"replicas,omitempty"`
	Image         string `json:"image,omitempty"`
}
```

其中Image、Replicas、EnableService为必须设置的属性，EnableIngress可以为空.

### 重新生成crd资源

```shell
make manifests
```

### 实现Reconcile逻辑

1. App的处理

```go
	logger := log.FromContext(ctx)
	app := &ingressv1beta1.App{}
	//从缓存中获取app
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
```

2. Deployment的处理

之前我们创建资源对象时，都是通过构造golang的struct来构造，但是对于复杂的资源对象
这样做费时费力，所以，我们可以先将资源定义为go template，然后替换需要修改的值之后，
反序列号为golang的struct对象，然后再通过client-go帮助我们创建或更新指定的资源。

我们的deployment、service、ingress都放在了controllers/template中，通过
utils来完成上述过程。

```go
	app := &ingressv1beta1.App{}
	if err := r.Get(ctx, req.NamespacedName, app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	deploy := utils.NewDeploy(app)

	if err := controllerutil.SetControllerReference(app, deploy, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	d := &appv1.Deployment{}
	err := r.Get(ctx, req.NamespacedName, d)
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if err != nil && errors.IsNotFound(err) {
		// Create Deployment
		if err := r.Create(ctx, deploy); err != nil {
			logger.Error(err, "create deployment failed")
			return ctrl.Result{}, err
		}
	}
	if err == nil {
		// Update Deploy
		//Bug: 这里会反复触发更新
		//原因：在187行SetupWithManager方法中，监听了Deployment，所以只要更新Deployment就会触发
		//     此处更新和controllerManager更新Deployment都会触发更新事件，导致循环触发
		//     这里只有Deployment的更新会触发App的Reconcile，Service和Ingress都不会触发，猜测的原因是Deployment Status的更新导致了该问题
		//修复方法：
		//方式1. 注释掉在148行SetupWithManager方法中对Deployment，Ingress，Service等的监听，该处的处理只是为了
		//      手动删除Deployment等后能够自动重建，但正常不会出现这种情况，是否需要根据情况而定
		//方式2. 加上判断条件，仅在app.Spec.Replicas != deployment.Spec.Replicas ||
		//      app.Spec.Image != deployment.Spec.Template.Spec.Containers[0].Image时才更新deployment
		//方式3. 添加Predicate，App的Spec发生变化时，才加入workqueue，例如:
		/* 这里的predicate.GenerationChangedPredicate{}表示update事件中如果对象app.metadata.generation没有变化，则不加入到workqueue中
		   而app.metadata.generation是一个int类型，当app.Spec发生变化，app.metadata.generation才会改变
		func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
			return ctrl.NewControllerManagedBy(mgr).
				For(&ingressv1beta1.App{}).
				Owns(&appv1.Deployment{}).
				Owns(&corev1.Service{}).
				Owns(&netv1.Ingress{}).
				WithEventFilter(predicate.GenerationChangedPredicate{}).
				Complete(r)
		}
		*/
		//		if *app.Spec.Replicas != *d.Spec.Replicas || app.Spec.Image != d.Spec.Template.Spec.Containers[0].Image {
		if err := r.Update(ctx, deploy); err != nil {
			logger.Error(err, "update deployment failed")
			return ctrl.Result{}, err
		}
		//		}
	}
```

3. Service的处理
```go
	svc := utils.NewService(app)
	if err := controllerutil.SetControllerReference(app, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	s := &corev1.Service{}
	// 从缓存中查找service对象
	err = r.Get(ctx, req.NamespacedName, s)
	// 遇到错误，返回
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	// 没找到service，并且enableSvc=true,创建service
	if err != nil && errors.IsNotFound(err) && *app.Spec.EnableSvc {
		// Create Service
		logger.Info("create service")
		if err := r.Create(ctx, svc); err != nil {
			logger.Error(err, "create service failed")
			return ctrl.Result{}, err
		}
	}
	// 找到service
	if err == nil {
		// 如果service enable,更新service
		if *app.Spec.EnableSvc {
			// Update Service
			if err := r.Update(ctx, svc); err != nil {
				logger.Error(err, "update service failed")
				return ctrl.Result{}, err
			}
			// 否则删除service
		} else {
			if err := r.Delete(ctx, s); err != nil {
				logger.Error(err, "delete service failed")
				return ctrl.Result{}, err
			}
		}
	}
```

4. Ingress的处理
```go
	// 当enableSvc为false时，直接返回，并检查是否存在ingress，如果存在，则删除
	if !*app.Spec.EnableSvc {
		//logger.Info("must enable service before create ingress")

		i := &netv1.Ingress{}
		err = r.Get(ctx, req.NamespacedName, i)
		if err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		} else if err != nil && errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		} else if err == nil {
			if err := r.Delete(ctx, i); err != nil {
				logger.Error(err, "delete ingress failed")
			}
		}
		return ctrl.Result{}, nil
	}

	ing := utils.NewIngress(app)

	if err := controllerutil.SetControllerReference(app, ing, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	i := &netv1.Ingress{}
	// 从缓存中查找ingress对象
	err = r.Get(ctx, req.NamespacedName, i)
	// 遇到错误，返回
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	// 没找到ingress，创建
	if err != nil && errors.IsNotFound(err) && *app.Spec.EnableIngress {
		// Create Ingress
		if err := r.Create(ctx, ing); err != nil {
			logger.Error(err, "create ingress failed")
			return ctrl.Result{}, err
		}
	}
	// 找到ingress
	if err == nil {
		// 如果ingress enable,更新ingress
		if *app.Spec.EnableIngress {
			// Update Ingress
			if err := r.Update(ctx, ing); err != nil {
				logger.Error(err, "update ingress failed")
				return ctrl.Result{}, err
			}
			// 否则删除ingress
		} else {
			if err := r.Delete(ctx, i); err != nil {
				logger.Error(err, "delete ingress failed")
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}
```

5. 删除service、ingress、deployment时，自动重建

```go
/*
   Owns表示，当Owns的资源发生create，update，delete事件时，会触发For资源的Reconcile，前提是Owns资源的OwnerReference为For资源
   在我们的例子中，当Deployment发生改变时，如在命令行中使用kubectl删除了Deployment，会触发App的Reconcile，重新创建Deployment

   而OwnerReference表明了资源对象的从属关系，在Deployment中，Pod的OwnerReference为ReplicaSet，ReplicaSet的OwnerReference为Deployment
   当一个资源对象的OwnerReference被删除时，该资源对象为孤儿状态，默认会被集群回收，这就解释了为什么我们删除Deployment时，ReplicaSet和Pod会被删除
   删除资源时可以使用--cascade指定级联删除方式，可用的选项为background，foreground，orphan
   background：删除主资源后立即返回，有系统回收子资源，默认值
   frontend：等待子资源删除后，在删除主资源
   orphan：只删除主资源，不删除子资源
   另外OwnerReference不能跨Namespace，即一个资源对象的OwnerReference只能在该资源对象的Namespace下
*/
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1beta1.App{}).
		Owns(&appv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&netv1.Ingress{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(r)
}
```

### Resource Templates(Go Templates)

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.ObjectMeta.Name}}
  namespace: {{.ObjectMeta.Namespace}}
  labels:
    app: {{.ObjectMeta.Name}}
spec:
  replicas: {{.Spec.Replicas}}
  selector:
    matchLabels:
      app: {{.ObjectMeta.Name}}
  template:
    metadata:
      labels:
        app: {{.ObjectMeta.Name}}
    spec:
      containers:
      - name: {{.ObjectMeta.Name}}
        image: {{.Spec.Image}}
        ports:
        - containerPort: 80

# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: {{.ObjectMeta.Name}}
  namespace: {{.ObjectMeta.Namespace}}
Spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: {{.ObjectMeta.Name}}

# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.ObjectMeta.Name}}
  namespace: {{.ObjectMeta.Namespace}}
Spec:
  ingressClassName: nginx 
  rules:
  - host: {{.ObjectMeta.Name}}.zq.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: {{.ObjectMeta.Name}}
            port:
              number: 80
```

### 解析Resource Templates并且将其反序列化为资源对象（Go Types）

```go
//utils/utils.go

package utils

import (
	"bytes"
	ingressv1beta1 "github.com/zyw/kubebuilder-demo/api/v1beta1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"text/template"
)

func parseTemplate(resource string, app *ingressv1beta1.App) []byte {
	// 解析模板
	tpl, err := template.ParseFiles("templates/" + resource + ".yml")
	if err != nil {
		panic(err)
	}
	b := new(bytes.Buffer)
	err = tpl.Execute(b, app)
	if err != nil {
		panic(err)
	}
	return b.Bytes()
}

func NewDeploy(app *ingressv1beta1.App) *appv1.Deployment {
	deploy := &appv1.Deployment{}
	if err := yaml.Unmarshal(parseTemplate("deployment", app), deploy); err != nil {
		panic(err)
	}
	return deploy
}

func NewService(app *ingressv1beta1.App) *corev1.Service {
	service := &corev1.Service{}
	if err := yaml.Unmarshal(parseTemplate("service", app), &service); err != nil {
		panic(err)
	}
	return service
}

func NewIngress(app *ingressv1beta1.App) *netv1.Ingress {
	ingress := &netv1.Ingress{}
	if err := yaml.Unmarshal(parseTemplate("ingress", app), &ingress); err != nil {
		panic(err)
	}
	return ingress
}

```

### 测试

#### 安装ingress controller

我们这里使用traefik作为ingress controller。

```shell
cat <<EOF>> traefik_values.yaml
ingressClass:
  enabled: true
  isDefaultClass: true #指定为默认的ingress
EOF

helm install traefik traefik/traefik -f traefik_values.yaml 
```

#### 安装crd

```shell
make install
```

#### 部署自定义controller

> 开发时可以直接在本地调试。

1. 构建镜像
```shell
IMG=hdss7-222.zq.com/clientgo-demo/app-controller make docker-build
```
2. push镜像
```shell
IMG=hdss7-222.zq.com/clientgo-demo/app-controller make docker-push
```

3. 部署
> fix: 部署之前需要修改一下controllers/app_controller.go的rbac
> ```yaml
> //+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
> //+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
> //+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
> ```
```shell
IMG=hdss7-222.zq.com/clientgo-demo/app-controller make deploy
```

#### 验证

1. 创建一个app

```shell
kubectl apply -f config/samples
```

2. 检查是否创建了deployment

3. 修改app，看service、ingress是否能被创建

4. 访问ingress，看是否可以访问到服务


### 遗留问题

1. enable_ingress默认为false, webhook将该值设置为反向值
2. 当设置enable_ingress为true时，enable_service必须设置为true

我们将通过admission webhook来解决。





# 使用kubebuilder实现webhook

### 参考项目

https://github.com/baidingtech/operator-lesson-demo/blob/main/kubebuilder-demo

### 需求

上节课我们实现了App的控制器的逻辑，但是我们希望在用户创建App资源时，
做一些更细的控制。


```yaml
apiVersion: ingress.baiding.tech/v1beta1
kind: App
metadata:
  name: app-sample
spec:
  image: nginx:latest
  replicas: 3
  enableIngress: false #默认值为false，需求为：设置为反向值;为true时，enable_service必须为true
  enableSvc: false
```

简单的校验我们可以直接使用CRD的scheme校验，但是复杂一点的需求我们
应该如何处理呢？

```yaml
//+kubebuilder:default:enable_ingress=false
```

我们这节课将会通过K8S提供的准入控制来实现。

### 准入控制

前面我们有学习到k8s提供了一系列的准入控制器，通过它们我们可以对api server的请求
进行处理。而对于我们自定义的需求，可以通过[MutatingAdmissionWebhook](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/admission-controllers/#mutatingadmissionwebhook)
和[ValidatingAdmissionWebhook](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook)
进行处理。

kubebuilder对其进行了支持，我们可以很方便的通过其实现我们的webhook逻辑。

### 创建webhook
> 创建webhook之前需要先创建api


1. 生成代码

```shell
kubebuilder create webhook --group ingress --version v1beta1 --kind App --defaulting --programmatic-validation
```

2. 生成manifests(config/webhook/manifests.yaml)

```shell
make manifests
```

创建之后，在main.go中会添加以下代码:

```go
	if err = (&ingressv1beta1.App{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "App")
		os.Exit(1)
	}
```

同时会生成下列文件，主要有：

- api/v1beta1/app_webhook.go webhook对应的handler，我们添加业务逻辑的地方
  
- api/v1beta1/webhook_suite_test.go 测试
  
- config/certmanager 自动生成自签名的证书，用于webhook server提供https服务

- config/webhook 用于注册webhook到k8s中

- config/crd/patches 为conversion自动注入caBoundle

- config/default/manager_webhook_patch.yaml 让manager的deployment支持webhook请求
- config/default/webhookcainjection_patch.yaml 为webhook server注入caBoundle

注入caBoundle由cert-manager的[ca-injector](https://cert-manager.io/docs/concepts/ca-injector/#examples) 组件实现

3. 修改配置

为了支持webhook，我们需要修改config/default/kustomization.yaml将相应的配置打开，具体可参考注释。
```yaml
# Adds namespace to all resources.
namespace: kubebuilder-demo-system

# Value of this field is prepended to the
# names of all resources, e.g. a deployment named
# "wordpress" becomes "alices-wordpress".
# Note that it should also match with the prefix (text before '-') of the namespace
# field above.
namePrefix: kubebuilder-demo-

# Labels to add to all resources and selectors.
#labels:
#- includeSelectors: true
#  pairs:
#    someName: someValue

resources:
- ../crd
- ../rbac
- ../manager
# [WEBHOOK] To enable webhook, uncomment all the sections with [WEBHOOK] prefix including the one in
# crd/kustomization.yaml
- ../webhook
# [CERTMANAGER] To enable cert-manager, uncomment all sections with 'CERTMANAGER'. 'WEBHOOK' components are required.
- ../certmanager
# [PROMETHEUS] To enable prometheus monitor, uncomment all sections with 'PROMETHEUS'.
#- ../prometheus

patches:
# Protect the /metrics endpoint by putting it behind auth.
# If you want your controller-manager to expose the /metrics
# endpoint w/o any authn/z, please comment the following line.
- path: manager_auth_proxy_patch.yaml

# [WEBHOOK] To enable webhook, uncomment all the sections with [WEBHOOK] prefix including the one in
# crd/kustomization.yaml
- path: manager_webhook_patch.yaml

# [CERTMANAGER] To enable cert-manager, uncomment all sections with 'CERTMANAGER'.
# Uncomment 'CERTMANAGER' sections in crd/kustomization.yaml to enable the CA injection in the admission webhooks.
# 'CERTMANAGER' needs to be enabled to use ca injection
- path: webhookcainjection_patch.yaml

# [CERTMANAGER] To enable cert-manager, uncomment all sections with 'CERTMANAGER' prefix.
# Uncomment the following replacements to add the cert-manager CA injection annotations
replacements:
  - source: # Add cert-manager annotation to ValidatingWebhookConfiguration, MutatingWebhookConfiguration and CRDs
      kind: Certificate
      group: cert-manager.io
      version: v1
      name: serving-cert # this name should match the one in certificate.yaml
      fieldPath: .metadata.namespace # namespace of the certificate CR
    targets:
      - select:
          kind: ValidatingWebhookConfiguration
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 0
          create: true
      - select:
          kind: MutatingWebhookConfiguration
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 0
          create: true
      - select:
          kind: CustomResourceDefinition
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 0
          create: true
  - source:
      kind: Certificate
      group: cert-manager.io
      version: v1
      name: serving-cert # this name should match the one in certificate.yaml
      fieldPath: .metadata.name
    targets:
      - select:
          kind: ValidatingWebhookConfiguration
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 1
          create: true
      - select:
          kind: MutatingWebhookConfiguration
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 1
          create: true
      - select:
          kind: CustomResourceDefinition
        fieldPaths:
          - .metadata.annotations.[cert-manager.io/inject-ca-from]
        options:
          delimiter: '/'
          index: 1
          create: true
  - source: # Add cert-manager annotation to the webhook Service
      kind: Service
      version: v1
      name: webhook-service
      fieldPath: .metadata.name # namespace of the service
    targets:
      - select:
          kind: Certificate
          group: cert-manager.io
          version: v1
        fieldPaths:
          - .spec.dnsNames.0
          - .spec.dnsNames.1
        options:
          delimiter: '.'
          index: 0
          create: true
  - source:
      kind: Service
      version: v1
      name: webhook-service
      fieldPath: .metadata.namespace # namespace of the service
    targets:
      - select:
          kind: Certificate
          group: cert-manager.io
          version: v1
        fieldPaths:
          - .spec.dnsNames.0
          - .spec.dnsNames.1
        options:
          delimiter: '.'
          index: 1
          create: true

```

### webhook业务逻辑

#### 设置enableIngress的默认值
```go
func (r *App) Default() {
	applog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	r.Spec.EnableIngress = !r.Spec.EnableIngress
}
```

#### 校验enableSvc的值

```go
func (r *App) ValidateCreate() error {
	applog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return r.validateApp()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *App) ValidateUpdate(old runtime.Object) error {
	applog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return r.validateApp()
}

func (r *App) validateApp() error {
	if !r.Spec.EnableService && r.Spec.EnableIngress {
		return apierrors.NewInvalid(GroupVersion.WithKind("App").GroupKind(), r.Name,
			field.ErrorList{
				field.Invalid(field.NewPath("enableSvc"),
					r.Spec.EnableService,
					"enable_service should be true when enable_ingress is true"),
			},
		)
	}
	return nil
}
```
### 测试

1. 安装cert-manager

```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.12.13/cert-manager.yaml
```

2. 部署

```shell
IMG=hdss7-222.zq.com/clientgo-demo/app-controller make deploy
```

3. 验证

```yaml
apiVersion: ingress.zq.com/v1beta1
kind: App
metadata:
  name: app-sample
spec:
  image: nginx:latest
  replicas: 3
  enable_ingress: false #会被修改为true
  enable_service: false #将会失败

```

```yaml
apiVersion: ingress.zq.com/v1beta1
kind: App
metadata:
  name: app-sample
spec:
  image: nginx:latest
  replicas: 3
  enable_ingress: false #会被修改为true
  enable_service: true #成功

```

```yaml
apiVersion: ingress.zq.com/v1beta1
kind: App
metadata:
  name: app-sample
spec:
  image: nginx:v1.13
  replicas: 3
  enable_ingress: true #会被修改为false
  enable_service: false #成功

```

### 如何本地测试

1. 添加本地测试相关的代码

- config/dev

- Makefile

```shell
.PHONY: dev
dev: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/dev | $(KUBECTL) apply -f -

.PHONY: undev
undev: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/dev | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -
```


2. 获取证书放到临时文件目录下

```shell
kubectl get secrets webhook-server-cert -n  kubebuilder-demo-system -o jsonpath='{..tls\.crt}' |base64 -d > certs/tls.crt
kubectl get secrets webhook-server-cert -n  kubebuilder-demo-system -o jsonpath='{..tls\.key}' |base64 -d > certs/tls.key
```

3. 修改main.go,让webhook server使用指定证书

```go
	if os.Getenv("ENVIRONMENT") == "DEV" {
		path, err := os.Getwd()
		if err != nil {
			setupLog.Error(err, "unable to get work dir")
			os.Exit(1)
		}
		webhookOptions = webhook.Options{
			TLSOpts: tlsOpts,
			CertDir: path + "/certs",
		}
	}
```

4. 部署

```shell
make dev
```

5. 清理环境

```shell
make undev
```

### 在集群中部署

1. 修改Dockerfile

```dockerfile
# Build the manager binary
FROM golang:1.22.12 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/controller/ internal/controller/
# Copy utils包和模板
COPY utils/ utils/
COPY templates/ templates/

# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.15.3
WORKDIR /
# Copy同时改变USER/GROUP
COPY --from=builder --chown=65532:65532 /workspace/manager .
COPY --from=builder --chown=65532:65532 /workspace/templates/ templates/
USER 65532:65532


ENTRYPOINT ["/manager"]
```

2. 制作镜像（由于go包无法下载，10.4.7.254:7890这是我本地的代理）这里省略了将hdss7-222.zq.com设置为insecure registry，以及/etc/hosts的配置

```shell
docker build --build-arg HTTPS_PROXY="http://10.4.7.254:7890" --build-arg HTTP_PROXY="http://10.4.7.254:7890" -t hdss7-222.zq.com/clientgo-demo/app-controller .
```

3. 上传镜像

```shell
docker push hdss7-222.zq.com/clientgo-demo/app-controller
```

部署

```shell
IMG=hdss7-222.zq.com/clientgo-demo/app-controller make deploy
```

