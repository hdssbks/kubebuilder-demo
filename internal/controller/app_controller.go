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
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	ingressv1beta1 "github.com/zyw/kubebuilder-demo/api/v1beta1"
)

// AppReconciler reconciles a App object
type AppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ingress.zq.com,resources=apps/finalizers,verbs=update

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

	deploy := utils.newDeploy(app)
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
		if err := r.Update(ctx, deploy); err != nil {
			logger.Error(err, "update deployment failed")
			return ctrl.Result{}, err
		}
	}

	svc := utils.newService(app)
	s := &corev1.Service{}
	// 从缓存中查找service对象
	err = r.Get(ctx, req.NamespacedName, s)
	// 遇到错误，返回
	if err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	// 没找到service，创建
	if err != nil && errors.IsNotFound(err) && *app.Spec.EnableSvc {
		// Create Service
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

	if !*app.Spec.EnableSvc {
		logger.Info("must enable service before create ingress")
		return ctrl.Result{}, nil
	}

	ing := utils.newIngress(app)
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
func (r *AppReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ingressv1beta1.App{}).
		Complete(r)
}
