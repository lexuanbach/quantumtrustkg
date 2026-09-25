// Cluster health observation for the commit step (Sect. 3, implementation).
//
// The paper's promotion rule says a profile is recorded as active only once both
// the PeerAuthentication and the DestinationRule of the edge exist and both
// workloads are ready. ClusterHealthObserver reads exactly that from the API
// server. It returns Unknown when either mesh object is missing or a workload has
// no pods, Healthy when at least one pod of each endpoint (label app=<service>)
// has the Ready condition, and Unhealthy otherwise. Only Healthy allows the
// runtime to promote, and Unknown keeps the edge in Applying.
//
// Limitation: readiness is judged per service by the app label, and one Ready pod
// is enough. The observer does not check that a sidecar has loaded the new
// configuration.

package mesh

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"quantumtrustkg/controller/pkg/types"
)

// ClusterHealthObserver implements the readiness check against a live cluster.
type ClusterHealthObserver struct {
	Client ctrlclient.Client
}

// NewClusterHealthObserver wraps a controller-runtime client.
func NewClusterHealthObserver(client ctrlclient.Client) *ClusterHealthObserver {
	return &ClusterHealthObserver{Client: client}
}

// ObserveEdgeHealth applies the readiness rule described in the file header.
// It uses a background context, because the runtime's health hook has no
// context parameter.
func (o *ClusterHealthObserver) ObserveEdgeHealth(edge types.CommunicationEdge) EdgeHealth {
	if o == nil || o.Client == nil {
		return EdgeHealthUnknown
	}
	ctx := context.Background()

	if !o.resourceExists(ctx, edge.Namespace, "PeerAuthentication", sanitizeName(edge.ID+"-mtls"), "security.istio.io/v1beta1") {
		return EdgeHealthUnknown
	}
	if !o.resourceExists(ctx, edge.Namespace, "DestinationRule", sanitizeName(edge.ID+"-tls"), "networking.istio.io/v1beta1") {
		return EdgeHealthUnknown
	}

	sourceReady, sourceKnown := o.workloadReady(ctx, edge.Namespace, edge.SourceService)
	destinationReady, destinationKnown := o.workloadReady(ctx, edge.Namespace, edge.DestinationService)
	if !sourceKnown || !destinationKnown {
		return EdgeHealthUnknown
	}
	if sourceReady && destinationReady {
		return EdgeHealthHealthy
	}
	return EdgeHealthUnhealthy
}

// resourceExists reports whether the named object can be read.
func (o *ClusterHealthObserver) resourceExists(ctx context.Context, namespace, kind, name, apiVersion string) bool {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(apiVersion)
	obj.SetKind(kind)
	if err := o.Client.Get(ctx, ctrlclient.ObjectKey{Namespace: namespace, Name: name}, obj); err != nil {
		return false
	}
	return true
}

// workloadReady lists the pods labelled app=<app>. known is false when the list
// fails or is empty. ready is true when at least one pod is Ready.
func (o *ClusterHealthObserver) workloadReady(ctx context.Context, namespace, app string) (ready bool, known bool) {
	list := &corev1.PodList{}
	if err := o.Client.List(ctx, list, ctrlclient.InNamespace(namespace), ctrlclient.MatchingLabelsSelector{
		Selector: labels.SelectorFromSet(map[string]string{"app": app}),
	}); err != nil {
		return false, false
	}
	if len(list.Items) == 0 {
		return false, false
	}
	for _, pod := range list.Items {
		if podReady(pod) {
			return true, true
		}
	}
	return false, true
}

// podReady returns the status of the pod's Ready condition.
func podReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
