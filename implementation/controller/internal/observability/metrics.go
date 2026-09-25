// Metrics hook (Sect. 3, implementation).
//
// MetricsRecorder is an interface with no implementation in this artifact, and
// nothing in the controller calls it. Limitation: the manager exposes the
// controller-runtime metrics endpoint only, and the paper's numbers come from the
// CSV outputs of the experiment commands and from controller logs. No
// metrics pipeline is involved.

package observability

// MetricsRecorder is the intended recording point for assignment outcomes.
type MetricsRecorder interface {
	RecordAssignmentResult(result string)
}
