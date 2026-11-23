package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"k8s-internal-platform/broker/internal/platform"
)

func diagnosticMetadata(name string, uid types.UID) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: ns, UID: uid,
		Labels:      map[string]string{appIDLabel: "app_1", "app.kubernetes.io/managed-by": "Helm", "app.kubernetes.io/part-of": "k8s-internal-platform"},
		Annotations: map[string]string{"meta.helm.sh/release-name": "app-app-1", "meta.helm.sh/release-namespace": ns}}
}

func diagnosticObjects() (*appsv1.Deployment, *appsv1.ReplicaSet, *corev1.Pod) {
	d := &appsv1.Deployment{ObjectMeta: diagnosticMetadata("app-app-1", "deployment")}
	set := &appsv1.ReplicaSet{ObjectMeta: diagnosticMetadata("replica-set", "replicaset")}
	set.OwnerReferences = []metav1.OwnerReference{{Kind: "Deployment", UID: d.UID, Controller: ptrBool(true)}}
	pod := &corev1.Pod{ObjectMeta: diagnosticMetadata("owned-pod", "pod"), Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "application"}}}}
	pod.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", UID: set.UID, Controller: ptrBool(true)}}
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "application", RestartCount: 1}}
	return d, set, pod
}

func TestDiagnosticLogsUseOwnedContainerAndBoundedStream(t *testing.T) {
	d, set, pod := diagnosticObjects()
	var logReads atomic.Int64
	var replacePod atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/apis/apps/v1/namespaces/" + ns + "/deployments":
			json.NewEncoder(w).Encode(appsv1.DeploymentList{Items: []appsv1.Deployment{*d}})
		case "/apis/apps/v1/namespaces/" + ns + "/replicasets":
			json.NewEncoder(w).Encode(appsv1.ReplicaSetList{Items: []appsv1.ReplicaSet{*set}})
		case "/api/v1/namespaces/" + ns + "/pods":
			current := pod.DeepCopy()
			if replacePod.Load() && logReads.Load() > 1 {
				current.UID = "replacement-pod"
			}
			json.NewEncoder(w).Encode(corev1.PodList{Items: []corev1.Pod{*current}})
		case "/api/v1/namespaces/" + ns + "/pods/owned-pod/log":
			logReads.Add(1)
			for key, value := range map[string]string{"container": "application", "previous": "true", "tailLines": "50", "sinceSeconds": "60", "limitBytes": "20", "timestamps": "true"} {
				if r.URL.Query().Get(key) != value {
					t.Errorf("query %s=%q", key, r.URL.Query().Get(key))
				}
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(strings.Repeat("log", 20)))
		default:
			t.Errorf("unexpected namespace/resource read: %s", r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	c := NewWithClientset(cs, ns)
	result, err := c.ReadLogs(context.Background(), "app_1", platform.LogOptions{Pod: "owned-pod", Previous: true, TailLines: 50, SinceSeconds: 60}, 20)
	if err != nil || len(result.Text) != 20 || !result.Truncated || logReads.Load() != 1 {
		t.Fatalf("logs=%+v err=%v reads=%d", result, err, logReads.Load())
	}
	if _, err := c.ReadLogs(context.Background(), "app_1", platform.LogOptions{Pod: "foreign-pod"}, 20); err == nil || logReads.Load() != 1 {
		t.Fatal("foreign Pod opened a log stream")
	}
	replacePod.Store(true)
	result, err = c.ReadLogs(context.Background(), "app_1", platform.LogOptions{Pod: "owned-pod", Previous: true, TailLines: 50, SinceSeconds: 60}, 20)
	if err == nil || result.Text != "" {
		t.Fatal("a replaced Pod name exposed unverified log output")
	}
}

func TestDiagnosticPodOwnershipRequiresControllerUIDChain(t *testing.T) {
	for _, scenario := range []string{"pod-owner", "set-owner", "deployment-release", "pod-namespace"} {
		t.Run(scenario, func(t *testing.T) {
			d, set, pod := diagnosticObjects()
			switch scenario {
			case "pod-owner":
				pod.OwnerReferences[0].UID = "foreign"
			case "set-owner":
				set.OwnerReferences[0].UID = "foreign"
			case "deployment-release":
				d.Annotations["meta.helm.sh/release-name"] = "foreign"
			case "pod-namespace":
				pod.Namespace = "kube-system"
			}
			c := NewWithClientset(fake.NewSimpleClientset(d, set, pod), ns)
			if _, err := c.ReadLogs(context.Background(), "app_1", platform.LogOptions{Pod: "owned-pod"}, 20); err == nil {
				t.Fatal("copied labels granted log access without ownership")
			}
		})
	}
}

func TestWorkloadEventsRejectForeignUIDsAndHonorLimit(t *testing.T) {
	d, set, pod := diagnosticObjects()
	owned := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "owned-event", Namespace: ns}, InvolvedObject: corev1.ObjectReference{Namespace: ns, UID: pod.UID}, Message: "readiness failed", Reason: "Unhealthy"}
	owned.Series = &corev1.EventSeries{Count: 3, LastObservedTime: metav1.NewMicroTime(time.Unix(100, 0))}
	foreign := owned.DeepCopy()
	foreign.Name = "foreign-event"
	foreign.InvolvedObject.UID = "foreign"
	foreign.Message = "foreign-message"
	c := NewWithClientset(fake.NewSimpleClientset(d, set, pod, owned, foreign), ns)
	result, err := c.ReadEvents(context.Background(), "app_1", 1)
	if err != nil || len(result.Items) != 1 || result.Items[0].Message != "readiness failed" || result.Items[0].Count != 3 {
		t.Fatalf("events=%+v error=%v", result, err)
	}
}

func TestRedactionReadsOnlyManagedSecretsAndFailsOnPagination(t *testing.T) {
	config := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "config", Namespace: ns, Labels: map[string]string{"app.kubernetes.io/managed-by": "k8s-internal-platform"}}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{"value": []byte("private-value")}}
	other := config.DeepCopy()
	other.Name = "foreign"
	other.Labels = map[string]string{}
	other.Data["value"] = []byte("unrelated-value")
	cs := fake.NewSimpleClientset(config, other)
	c := NewWithClientset(cs, ns)
	values, err := c.RedactionValues(context.Background())
	if err != nil || len(values) != 1 || string(values[0]) != "private-value" {
		t.Fatal("unrelated Secret was included in diagnostics")
	}
	for i := 0; i < 101; i++ {
		item := config.DeepCopy()
		item.Name = string(rune('a'+i)) + "-config"
		cs.CoreV1().Secrets(ns).Create(context.Background(), item, metav1.CreateOptions{})
	}
	if _, err := c.RedactionValues(context.Background()); err == nil {
		t.Fatal("unbounded Secret listing accepted")
	}
}
