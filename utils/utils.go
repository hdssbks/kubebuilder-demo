package utils

import (
	ingressv1beta1 "github.com/zyw/kubebuilder-demo/api/v1beta1"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
)

func NewDeploy(app *ingressv1beta1.App) *appv1.Deployment {

}

func NewService(app *ingressv1beta1.App) *corev1.Service {

}

func NewIngress(app *ingressv1beta1.App) *netv1.Ingress {

}
