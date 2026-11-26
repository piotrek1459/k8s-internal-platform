package cluster

import (
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"

	"k8s-internal-platform/broker/internal/platform"
)

// CompareManifest reports chart-owned differences only. Rollout health and
// server-allocated addresses/defaults are separate from configuration drift.
func (c *Client) CompareManifest(ctx context.Context, appID, manifest string) ([]platform.DriftDifference, error) {
	items := []platform.DriftDifference{}
	expected := map[string]bool{}
	decoder := yaml.NewYAMLOrJSONDecoder(strings.NewReader(manifest), 4096)
	deployments, services, ingresses := 0, 0, 0
	for {
		var raw map[string]any
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, platform.Errorf(platform.CodeConflict, "recorded manifest is invalid")
		}
		if len(raw) == 0 {
			continue
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return nil, platform.Errorf(platform.CodeConflict, "recorded manifest is invalid")
		}
		kind, _ := raw["kind"].(string)
		var metadata struct {
			Metadata metav1.ObjectMeta `json:"metadata"`
		}
		if json.Unmarshal(body, &metadata) != nil || metadata.Metadata.Namespace != c.namespace || metadata.Metadata.Name != releaseFor(appID) || metadata.Metadata.Labels[appIDLabel] != appID {
			return nil, platform.Errorf(platform.CodeConflict, "recorded resource ownership is invalid")
		}
		resource := kind + "/" + metadata.Metadata.Name
		if expected[resource] {
			return nil, platform.Errorf(platform.CodeConflict, "recorded resource identity is ambiguous")
		}
		expected[resource] = true
		add := func(field string, want, got any) {
			if !equality.Semantic.DeepEqual(want, got) {
				items = append(items, platform.DriftDifference{Resource: resource, Field: field, Change: "modified"})
			}
		}
		missing := func(err error) bool {
			if apierrors.IsNotFound(err) {
				items = append(items, platform.DriftDifference{Resource: resource, Field: "/", Change: "missing"})
				return true
			}
			return false
		}
		ownership := func(meta metav1.ObjectMeta) bool {
			if !c.owned(meta, appID) {
				items = append(items, platform.DriftDifference{Resource: resource, Field: "/metadata/ownership", Change: "modified"})
				return false
			}
			for key, value := range metadata.Metadata.Labels {
				add("/metadata/labels/"+pointerKey(key), value, meta.Labels[key])
			}
			return true
		}
		switch kind {
		case "Deployment":
			deployments++
			var want appsv1.Deployment
			if json.Unmarshal(body, &want) != nil {
				return nil, diagnosticUnavailable()
			}
			got, err := c.cs.AppsV1().Deployments(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if missing(err) {
				continue
			}
			if err != nil {
				return nil, diagnosticUnavailable()
			}
			if !ownership(got.ObjectMeta) {
				continue
			}
			add("/spec/replicas", want.Spec.Replicas, got.Spec.Replicas)
			add("/spec/selector", want.Spec.Selector, got.Spec.Selector)
			add("/spec/strategy", want.Spec.Strategy, got.Spec.Strategy)
			for key, value := range want.Spec.Template.Labels {
				add("/spec/template/metadata/labels/"+pointerKey(key), value, got.Spec.Template.Labels[key])
			}
			add("/spec/template/metadata/annotations/platform.k8s.io~1restart-token", want.Spec.Template.Annotations["platform.k8s.io/restart-token"], got.Spec.Template.Annotations["platform.k8s.io/restart-token"])
			add("/spec/template/spec/automountServiceAccountToken", want.Spec.Template.Spec.AutomountServiceAccountToken, got.Spec.Template.Spec.AutomountServiceAccountToken)
			add("/spec/template/spec/enableServiceLinks", want.Spec.Template.Spec.EnableServiceLinks, got.Spec.Template.Spec.EnableServiceLinks)
			add("/spec/template/spec/securityContext", want.Spec.Template.Spec.SecurityContext, got.Spec.Template.Spec.SecurityContext)
			add("/spec/template/spec/imagePullSecrets", want.Spec.Template.Spec.ImagePullSecrets, got.Spec.Template.Spec.ImagePullSecrets)
			add("/spec/template/spec/volumes", want.Spec.Template.Spec.Volumes, got.Spec.Template.Spec.Volumes)
			add("/spec/template/spec/terminationGracePeriodSeconds", want.Spec.Template.Spec.TerminationGracePeriodSeconds, got.Spec.Template.Spec.TerminationGracePeriodSeconds)
			if len(want.Spec.Template.Spec.Containers) != 1 || len(got.Spec.Template.Spec.Containers) != 1 || got.Spec.Template.Spec.Containers[0].Name != "application" {
				items = append(items, platform.DriftDifference{Resource: resource, Field: "/spec/template/spec/containers", Change: "modified"})
				continue
			}
			defaultDiagnosticProbes(&want.Spec.Template)
			defaultDiagnosticProbes(&got.Spec.Template)
			w, g := want.Spec.Template.Spec.Containers[0], got.Spec.Template.Spec.Containers[0]
			prefix := "/spec/template/spec/containers/application/"
			add(prefix+"image", w.Image, g.Image)
			add(prefix+"imagePullPolicy", w.ImagePullPolicy, g.ImagePullPolicy)
			add(prefix+"command", w.Command, g.Command)
			add(prefix+"args", w.Args, g.Args)
			add(prefix+"env", w.Env, g.Env)
			add(prefix+"envFrom", w.EnvFrom, g.EnvFrom)
			add(prefix+"ports", w.Ports, g.Ports)
			add(prefix+"resources", w.Resources, g.Resources)
			add(prefix+"startupProbe", w.StartupProbe, g.StartupProbe)
			add(prefix+"readinessProbe", w.ReadinessProbe, g.ReadinessProbe)
			add(prefix+"livenessProbe", w.LivenessProbe, g.LivenessProbe)
			add(prefix+"lifecycle", w.Lifecycle, g.Lifecycle)
			add(prefix+"securityContext", w.SecurityContext, g.SecurityContext)
			add(prefix+"volumeMounts", w.VolumeMounts, g.VolumeMounts)
		case "Service":
			services++
			var want corev1.Service
			if json.Unmarshal(body, &want) != nil {
				return nil, diagnosticUnavailable()
			}
			got, err := c.cs.CoreV1().Services(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if missing(err) {
				continue
			}
			if err != nil {
				return nil, diagnosticUnavailable()
			}
			if !ownership(got.ObjectMeta) {
				continue
			}
			add("/spec/type", want.Spec.Type, got.Spec.Type)
			add("/spec/selector", want.Spec.Selector, got.Spec.Selector)
			add("/spec/ports", want.Spec.Ports, got.Spec.Ports)
		case "Ingress":
			ingresses++
			var want networkingv1.Ingress
			if json.Unmarshal(body, &want) != nil {
				return nil, diagnosticUnavailable()
			}
			got, err := c.cs.NetworkingV1().Ingresses(c.namespace).Get(ctx, want.Name, metav1.GetOptions{})
			if missing(err) {
				continue
			}
			if err != nil {
				return nil, diagnosticUnavailable()
			}
			if !ownership(got.ObjectMeta) {
				continue
			}
			add("/spec", want.Spec, got.Spec)
			for _, key := range []string{"traefik.ingress.kubernetes.io/router.entrypoints", "traefik.ingress.kubernetes.io/router.tls"} {
				add("/metadata/annotations/"+pointerKey(key), want.Annotations[key], got.Annotations[key])
			}
		default:
			return nil, platform.Errorf(platform.CodeConflict, "recorded resource kind is unsupported")
		}
	}
	if deployments != 1 || services != 1 || ingresses > 1 {
		return nil, platform.Errorf(platform.CodeConflict, "recorded workload/route count is invalid")
	}
	options := metav1.ListOptions{LabelSelector: appIDLabel + "=" + appID, Limit: diagnosticResourceLimit + 1}
	extra := func(kind string, meta metav1.ObjectMeta) {
		key := kind + "/" + meta.Name
		if c.owned(meta, appID) && !expected[key] {
			items = append(items, platform.DriftDifference{Resource: key, Field: "/", Change: "unexpected"})
		}
	}
	ds, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, options)
	if err != nil || ds.Continue != "" || len(ds.Items) > diagnosticResourceLimit {
		return nil, diagnosticUnavailable()
	}
	for _, d := range ds.Items {
		extra("Deployment", d.ObjectMeta)
	}
	svcs, err := c.cs.CoreV1().Services(c.namespace).List(ctx, options)
	if err != nil || svcs.Continue != "" || len(svcs.Items) > diagnosticResourceLimit {
		return nil, diagnosticUnavailable()
	}
	for _, svc := range svcs.Items {
		extra("Service", svc.ObjectMeta)
	}
	routes, err := c.cs.NetworkingV1().Ingresses(c.namespace).List(ctx, options)
	if err != nil || routes.Continue != "" || len(routes.Items) > diagnosticResourceLimit {
		return nil, diagnosticUnavailable()
	}
	for _, route := range routes.Items {
		extra("Ingress", route.ObjectMeta)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Resource == items[j].Resource {
			return items[i].Field < items[j].Field
		}
		return items[i].Resource < items[j].Resource
	})
	return items, nil
}

func pointerKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
}

func defaultDiagnosticProbes(template *corev1.PodTemplateSpec) {
	defaultProbeThresholds(template)
	for i := range template.Spec.Containers {
		container := &template.Spec.Containers[i]
		for _, probe := range []*corev1.Probe{container.StartupProbe, container.ReadinessProbe, container.LivenessProbe} {
			if probe != nil && probe.HTTPGet != nil && probe.HTTPGet.Scheme == "" {
				probe.HTTPGet.Scheme = corev1.URISchemeHTTP
			}
		}
	}
}
