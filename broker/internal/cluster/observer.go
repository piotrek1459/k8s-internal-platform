package cluster

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s-internal-platform/broker/internal/platform"
)

// appIDLabel is the ownership label the application chart stamps on every managed
// resource (see deploy/helm/application/templates/_helpers.tpl). The observer
// selects an application's workload by this label rather than by a guessed name.
const appIDLabel = "platform.local/app-id"

var _ platform.ClusterObserver = (*Client)(nil)

// Observe reports the current workload status for an application by reading its
// Deployment and Service endpoints. A cluster read failure is returned as a
// domain error so the caller can mark cached status stale rather than overwrite
// it as fresh.
func (c *Client) Observe(ctx context.Context, appID string) (platform.ObservedStatus, error) {
	selector := appIDLabel + "=" + appID
	deployments, err := c.cs.AppsV1().Deployments(c.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return platform.ObservedStatus{}, platform.Errorf(platform.CodeDependencyUnavailable,
			"could not read workloads: %v", err)
	}
	now := metav1.Now().Time.UTC()

	if len(deployments.Items) == 0 {
		return platform.ObservedStatus{Phase: platform.WorkloadUnknown, ObservedAt: now}, nil
	}
	d := deployments.Items[0]

	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	ready := d.Status.ReadyReplicas

	status := platform.ObservedStatus{
		DesiredReplicas:    int(desired),
		ReadyReplicas:      int(ready),
		ObservedGeneration: d.Status.ObservedGeneration,
		ObservedAt:         now,
	}

	// Endpoint readiness confirms the Service actually has ready backends, which a
	// replica count alone does not guarantee. A read *failure* (as opposed to an
	// empty, not-yet-ready result) is a cluster-availability problem and is
	// propagated so the caller marks status stale rather than reporting a fresh
	// routeReady=false.
	routeReady, err := c.hasReadyEndpoints(ctx, appID)
	if err != nil {
		return platform.ObservedStatus{}, platform.Errorf(platform.CodeDependencyUnavailable,
			"could not read service endpoints: %v", err)
	}
	status.RouteReady = routeReady
	status.Phase, status.WorkloadReady = classify(d, desired, ready)
	return status, nil
}

// classify derives a workload phase from the Deployment's desired/ready replicas
// and rollout progress. Zero desired replicas is the Stopped state; a rollout
// still converging is Deploying/Updating; full readiness is Ready.
func classify(d appsv1.Deployment, desired, ready int32) (platform.WorkloadPhase, bool) {
	if desired == 0 {
		return platform.WorkloadStopped, false
	}
	// A spec change not yet observed by the controller, or fewer ready than
	// desired, means the rollout is still in progress.
	if d.Status.ObservedGeneration < d.Generation {
		return platform.WorkloadUpdating, false
	}
	if ready >= desired {
		return platform.WorkloadReady, true
	}
	if ready == 0 {
		return platform.WorkloadDeploying, false
	}
	return platform.WorkloadDegraded, false
}

// hasReadyEndpoints reports whether the application's Service has at least one
// ready backend. It returns an error only when the cluster reads themselves fail,
// so a genuinely empty/not-ready result (false, nil) is distinguished from an
// unavailable cluster (false, err).
func (c *Client) hasReadyEndpoints(ctx context.Context, appID string) (bool, error) {
	// Find the application's Service by its ownership label, then list the
	// EndpointSlices Kubernetes maintains for that Service (labelled with the
	// service name). This avoids assuming the Service's name from the app ID.
	services, err := c.cs.CoreV1().Services(c.namespace).List(ctx, metav1.ListOptions{LabelSelector: appIDLabel + "=" + appID})
	if err != nil {
		return false, err
	}
	if len(services.Items) == 0 {
		return false, nil // no Service yet; not an error
	}
	svcName := services.Items[0].Name
	slices, err := c.cs.DiscoveryV1().EndpointSlices(c.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "kubernetes.io/service-name=" + svcName,
	})
	if err != nil {
		return false, err
	}
	for _, s := range slices.Items {
		for _, ep := range s.Endpoints {
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				return true, nil
			}
		}
	}
	return false, nil
}
