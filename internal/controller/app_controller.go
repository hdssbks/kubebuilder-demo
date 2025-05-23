/*
Copyright 2025.

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

package controller

import (
	"context"
	"github.com/hdssbks/kubebuilder-demo/utils"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	ingressv1beta1 "github.com/hdssbks/kubebuilder-demo/api/v1beta1"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// 添加事件上报机制，可以通过kubectl describe app app-sample查看到事件
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// 由于我们的controller需要操作deployments，services，ingresses，当controller部署至集群中时，需要相应的权限，故需要添加相应的rbac
// 添加完成后需要使用make manifests以创建rbac的manifests

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the App object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *AppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// 从缓存中获取app对象，如果没找到，表示删除事件，直接返回
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
			// 写入事件
			r.Recorder.Event(app, corev1.EventTypeWarning, "CreateDeploymentFailed", err.Error())
			return ctrl.Result{}, err
		}
		r.Recorder.Event(app, corev1.EventTypeNormal, "CreateDeploymentSuccess", "Create deployment success")
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

// SetupWithManager sets up the controller with the Manager.
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
