// Assignment event emission (Sect. 3, implementation).
//
// Each decision of the control loop produces one event line, such as
// assignment_promoted, assignment_blocked, assignment_deferred,
// assignment_applying or assignment_failed, together with the edge ID and the
// reason string. The lines go to the standard logger in a key=value form. The
// reason is the same string that appears in the AssignmentStatus, which means a log line
// and a status object can be matched. This file backs the auditable-status
// claim of Sect. 3 at the log level.

package observability

import "log"

// EmitEvent logs an event without edge context.
func EmitEvent(event string) {
	log.Printf("event=%s\n", event)
}

// EmitAssignmentEvent logs an event for one edge with the reason for the outcome.
func EmitAssignmentEvent(event, edgeID, reason string) {
	log.Printf("event=%s edge=%s reason=%s\n", event, edgeID, reason)
}
