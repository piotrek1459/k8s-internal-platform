package cluster

import (
	"context"
	"encoding/json"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestVerifyManifestRejectsLiveDriftAndUnreadyGeneration(t *testing.T) {
	ctx := context.Background()
	d := &appsv1.Deployment{TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}, ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns, Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: ptrInt32(1), Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "example.com/app@sha256:good"}}}}}, Status: appsv1.DeploymentStatus{ObservedGeneration: 2, ReadyReplicas: 1, UpdatedReplicas: 1, Replicas: 1}}
	svc := &corev1.Service{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"}, ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: ns}, Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, Selector: map[string]string{"app": "owned"}, Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(8080)}}}}
	first, _ := json.Marshal(d)
	second, _ := json.Marshal(svc)
	manifest := string(first) + "\n---\n" + string(second)
	cs := fake.NewSimpleClientset(d, svc)
	client := NewWithClientset(cs, ns)
	if err := client.VerifyManifest(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	d.Spec.Template.Spec.Containers[0].Image = "example.com/app@sha256:wrong"
	cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
	if err := client.VerifyManifest(ctx, manifest); err == nil {
		t.Fatal("different live image accepted")
	}
	d.Spec.Template.Spec.Containers[0].Image = "example.com/app@sha256:good"
	d.Status.ObservedGeneration = 1
	cs.AppsV1().Deployments(ns).Update(ctx, d, metav1.UpdateOptions{})
	if err := client.VerifyManifest(ctx, manifest); err == nil {
		t.Fatal("unobserved live generation accepted")
	}
}
