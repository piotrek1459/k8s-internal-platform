package cluster

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s-internal-platform/broker/internal/platform"
)

const diagnosticResourceLimit = 50

func diagnosticUnavailable() error {
	return platform.Errorf(platform.CodeDependencyUnavailable, "workload diagnostics are unavailable or exceed their read limits")
}

func releaseFor(appID string) string { return "app-" + sanitizeID(appID) }

func (c *Client) owned(meta metav1.ObjectMeta, appID string) bool {
	return meta.Namespace == c.namespace && meta.UID != "" && meta.Labels[appIDLabel] == appID &&
		meta.Labels["app.kubernetes.io/managed-by"] == "Helm" &&
		meta.Labels["app.kubernetes.io/part-of"] == "k8s-internal-platform" &&
		meta.Annotations["meta.helm.sh/release-name"] == releaseFor(appID) &&
		meta.Annotations["meta.helm.sh/release-namespace"] == c.namespace
}

func controllerIs(refs []metav1.OwnerReference, kind string, uid types.UID) bool {
	for _, ref := range refs {
		if ref.Kind == kind && ref.UID == uid && ref.Controller != nil && *ref.Controller {
			return true
		}
	}
	return false
}

// ownedPods checks Deployment -> ReplicaSet -> Pod UIDs, not just copied labels.
func (c *Client) ownedPods(ctx context.Context, appID string) (*appsv1.Deployment, []corev1.Pod, []appsv1.ReplicaSet, error) {
	options := metav1.ListOptions{LabelSelector: appIDLabel + "=" + appID, Limit: diagnosticResourceLimit + 1}
	deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, options)
	if err != nil || deployments.Continue != "" || len(deployments.Items) > diagnosticResourceLimit {
		return nil, nil, nil, diagnosticUnavailable()
	}
	var deployment *appsv1.Deployment
	for i := range deployments.Items {
		d := &deployments.Items[i]
		if c.owned(d.ObjectMeta, appID) {
			if deployment != nil {
				return nil, nil, nil, platform.Errorf(platform.CodeConflict, "application workload ownership is ambiguous")
			}
			deployment = d
		}
	}
	if deployment == nil {
		return nil, nil, nil, platform.Errorf(platform.CodeNotFound, "no owned workload is available")
	}
	sets, err := c.cs.AppsV1().ReplicaSets(c.namespace).List(ctx, options)
	if err != nil || sets.Continue != "" || len(sets.Items) > diagnosticResourceLimit {
		return nil, nil, nil, diagnosticUnavailable()
	}
	setIDs := map[types.UID]bool{}
	ownedSets := []appsv1.ReplicaSet{}
	for _, set := range sets.Items {
		if set.Namespace == c.namespace && set.UID != "" && set.Labels[appIDLabel] == appID && controllerIs(set.OwnerReferences, "Deployment", deployment.UID) {
			setIDs[set.UID] = true
			ownedSets = append(ownedSets, set)
		}
	}
	pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, options)
	if err != nil || pods.Continue != "" || len(pods.Items) > diagnosticResourceLimit {
		return nil, nil, nil, diagnosticUnavailable()
	}
	ownedPods := []corev1.Pod{}
	for _, pod := range pods.Items {
		if pod.Namespace != c.namespace || pod.UID == "" || pod.Labels[appIDLabel] != appID {
			continue
		}
		for id := range setIDs {
			if controllerIs(pod.OwnerReferences, "ReplicaSet", id) {
				ownedPods = append(ownedPods, pod)
				break
			}
		}
	}
	sort.Slice(ownedPods, func(i, j int) bool {
		if ownedPods[i].CreationTimestamp.Equal(&ownedPods[j].CreationTimestamp) {
			return ownedPods[i].Name < ownedPods[j].Name
		}
		return ownedPods[i].CreationTimestamp.After(ownedPods[j].CreationTimestamp.Time)
	})
	return deployment, ownedPods, ownedSets, nil
}

func (c *Client) ReadLogs(ctx context.Context, appID string, options platform.LogOptions, byteLimit int64) (platform.LogResult, error) {
	original, pods, _, err := c.ownedPods(ctx, appID)
	if err != nil {
		return platform.LogResult{}, err
	}
	var pod *corev1.Pod
	for i := range pods {
		if options.Pod != "" && pods[i].Name != options.Pod {
			continue
		}
		if options.Pod == "" && pods[i].DeletionTimestamp != nil {
			continue
		}
		pod = &pods[i]
		break
	}
	if pod == nil {
		return platform.LogResult{}, platform.Errorf(platform.CodeNotFound, "the selected application Pod is unavailable")
	}
	container := false
	for _, item := range pod.Spec.Containers {
		if item.Name == "application" {
			container = true
		}
	}
	if !container {
		return platform.LogResult{}, platform.Errorf(platform.CodeConflict, "the application container is unavailable")
	}
	if options.Previous {
		available := false
		for _, status := range pod.Status.ContainerStatuses {
			if status.Name == "application" && status.RestartCount > 0 {
				available = true
			}
		}
		if !available {
			return platform.LogResult{}, platform.Errorf(platform.CodeNotFound, "no previous application container logs are available")
		}
	}
	stream, err := c.cs.CoreV1().Pods(c.namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "application", Previous: options.Previous, TailLines: &options.TailLines,
		SinceSeconds: &options.SinceSeconds, LimitBytes: &byteLimit, Timestamps: true,
	}).Stream(ctx)
	if err != nil {
		return platform.LogResult{}, diagnosticUnavailable()
	}
	defer stream.Close()
	body, err := io.ReadAll(io.LimitReader(stream, byteLimit+1))
	if err != nil {
		return platform.LogResult{}, diagnosticUnavailable()
	}
	_ = stream.Close()
	// A Pod name can be reused after deletion. Revalidate the controller UID
	// chain after reading, before exposing any bytes from the name-based log API.
	current, candidates, _, err := c.ownedPods(ctx, appID)
	if err != nil || current.UID != original.UID {
		return platform.LogResult{}, platform.Errorf(platform.CodeConflict, "workload ownership changed during the log read; refresh")
	}
	stillOwned := false
	for _, candidate := range candidates {
		if candidate.Name == pod.Name && candidate.UID == pod.UID {
			stillOwned = true
		}
	}
	if !stillOwned {
		return platform.LogResult{}, platform.Errorf(platform.CodeConflict, "Pod ownership changed during the log read; refresh")
	}
	truncated := int64(len(body)) >= byteLimit
	if int64(len(body)) > byteLimit {
		body = body[:byteLimit]
	}
	return platform.LogResult{Pod: pod.Name, Container: "application", Text: string(body), Truncated: truncated, ObservedAt: time.Now().UTC()}, nil
}

func (c *Client) ReadEvents(ctx context.Context, appID string, limit int) (platform.EventResult, error) {
	d, pods, sets, err := c.ownedPods(ctx, appID)
	if err != nil {
		return platform.EventResult{}, err
	}
	refs := map[types.UID]string{d.UID: "Deployment/" + d.Name}
	for _, pod := range pods {
		refs[pod.UID] = "Pod/" + pod.Name
	}
	for _, set := range sets {
		refs[set.UID] = "ReplicaSet/" + set.Name
	}
	options := metav1.ListOptions{LabelSelector: appIDLabel + "=" + appID, Limit: diagnosticResourceLimit + 1}
	services, err := c.cs.CoreV1().Services(c.namespace).List(ctx, options)
	if err != nil || services.Continue != "" || len(services.Items) > diagnosticResourceLimit {
		return platform.EventResult{}, diagnosticUnavailable()
	}
	for _, svc := range services.Items {
		if c.owned(svc.ObjectMeta, appID) {
			refs[svc.UID] = "Service/" + svc.Name
		}
	}
	ingresses, err := c.cs.NetworkingV1().Ingresses(c.namespace).List(ctx, options)
	if err != nil || ingresses.Continue != "" || len(ingresses.Items) > diagnosticResourceLimit {
		return platform.EventResult{}, diagnosticUnavailable()
	}
	for _, ingress := range ingresses.Items {
		if c.owned(ingress.ObjectMeta, appID) {
			refs[ingress.UID] = "Ingress/" + ingress.Name
		}
	}
	if len(refs) > diagnosticResourceLimit {
		return platform.EventResult{}, diagnosticUnavailable()
	}
	result := platform.EventResult{Items: []platform.WorkloadEvent{}, ObservedAt: time.Now().UTC()}
	// UID field selectors avoid downloading another application's diagnostics.
	for uid, resource := range refs {
		events, err := c.cs.CoreV1().Events(c.namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.uid=" + string(uid), Limit: 21})
		if err != nil {
			return platform.EventResult{}, diagnosticUnavailable()
		}
		result.Truncated = result.Truncated || events.Continue != "" || len(events.Items) > 20
		for _, event := range events.Items[:min(len(events.Items), 20)] {
			if event.Namespace != c.namespace || event.InvolvedObject.Namespace != c.namespace || event.InvolvedObject.UID != uid {
				continue
			}
			at := event.LastTimestamp.Time
			if !event.EventTime.IsZero() {
				at = event.EventTime.Time
			}
			if event.Series != nil {
				at = event.Series.LastObservedTime.Time
			}
			if at.IsZero() {
				at = event.CreationTimestamp.Time
			}
			count := event.Count
			if event.Series != nil {
				count = event.Series.Count
			}
			result.Items = append(result.Items, platform.WorkloadEvent{At: at.UTC(), Type: event.Type, Reason: event.Reason, Message: event.Message, Resource: resource, Count: count})
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		if a.At.Equal(b.At) {
			if a.Resource == b.Resource {
				return a.Reason < b.Reason
			}
			return a.Resource < b.Resource
		}
		return a.At.After(b.At)
	})
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
		result.Truncated = true
	}
	return result, nil
}

// RedactionValues reads only platform-managed configuration/authentication
// Secrets. Helm release Secrets and unrelated namespace Secrets are excluded.
func (c *Client) RedactionValues(ctx context.Context) ([][]byte, error) {
	values := [][]byte{}
	budget := 0
	for _, selector := range []string{"app.kubernetes.io/managed-by=k8s-internal-platform", "platform.local/credential=true"} {
		secrets, err := c.cs.CoreV1().Secrets(c.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector, Limit: 101})
		if err != nil || secrets.Continue != "" || len(secrets.Items) > 100 {
			return nil, diagnosticUnavailable()
		}
		for _, secret := range secrets.Items {
			if secret.Namespace != c.namespace {
				continue
			}
			if selector == "app.kubernetes.io/managed-by=k8s-internal-platform" && (secret.Labels["app.kubernetes.io/managed-by"] != "k8s-internal-platform" || secret.Type != corev1.SecretTypeOpaque) {
				continue
			}
			if selector == "platform.local/credential=true" && (secret.Labels["platform.local/credential"] != "true" || secret.Type != corev1.SecretTypeDockerConfigJson) {
				continue
			}
			for _, value := range secret.Data {
				budget += len(value)
				if budget > 512*1024 {
					return nil, diagnosticUnavailable()
				}
				values = append(values, value)
			}
			if secret.Type == corev1.SecretTypeDockerConfigJson {
				var config struct {
					Auths map[string]struct {
						Password string `json:"password"`
						Auth     string `json:"auth"`
					} `json:"auths"`
				}
				if json.Unmarshal(secret.Data[corev1.DockerConfigJsonKey], &config) != nil {
					return nil, diagnosticUnavailable()
				}
				for _, auth := range config.Auths {
					values = append(values, []byte(auth.Password), []byte(auth.Auth))
					if auth.Auth != "" {
						decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
						if err != nil {
							return nil, diagnosticUnavailable()
						}
						values = append(values, decoded)
						if _, password, ok := strings.Cut(string(decoded), ":"); ok {
							values = append(values, []byte(password))
						}
					}
				}
			}
		}
	}
	return values, nil
}
