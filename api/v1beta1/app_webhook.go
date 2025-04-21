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

package v1beta1

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var applog = logf.Log.WithName("app-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *App) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-ingress-zq-com-v1beta1-app,mutating=true,failurePolicy=fail,sideEffects=None,groups=ingress.zq.com,resources=apps,verbs=create;update,versions=v1beta1,name=mapp.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &App{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
// 这里是MutatingAdmissionWebhook的逻辑，通常用来设置默认值，设置后会交给ValidatingAdmissionWebhook校验
func (r *App) Default() {
	applog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
	*r.Spec.EnableIngress = !*r.Spec.EnableIngress
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// 在使用kubebuilder create webhook后，需要使用make manifests以创建webhook的manifests
//+kubebuilder:webhook:path=/validate-ingress-zq-com-v1beta1-app,mutating=false,failurePolicy=fail,sideEffects=None,groups=ingress.zq.com,resources=apps,verbs=create;update,versions=v1beta1,name=vapp.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &App{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
// 这里是ValidatingAdmissionWebhook的逻辑，当App资源被创建时，会被这里拦截校验
func (r *App) ValidateCreate() (admission.Warnings, error) {
	applog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.

	return r.validApp()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
// 这里是ValidatingAdmissionWebhook的逻辑，当App资源被更新时，会被这里拦截校验
func (r *App) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	applog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return r.validApp()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
// 这里是ValidatingAdmissionWebhook的逻辑，当App资源被删除时，会被这里拦截校验
func (r *App) ValidateDelete() (admission.Warnings, error) {
	applog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (r *App) validApp() (admission.Warnings, error) {
	if !*r.Spec.EnableSvc && *r.Spec.EnableIngress {
		return nil, errors.NewInvalid(GroupVersion.WithKind("App").GroupKind(), r.Name, field.ErrorList{
			field.Invalid(field.NewPath("enableSvc"), r.Spec.EnableSvc, "must enable svc before enable ingress"),
		})
	}
	return nil, nil
}
