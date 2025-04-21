package utils

import (
	"bytes"
	ingressv1beta1 "github.com/hdssbks/kubebuilder-demo/api/v1beta1"
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
