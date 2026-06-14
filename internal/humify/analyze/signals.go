package analyze

// Signal names are the canonical set of finding signals every detector can emit.
// They are the single source of truth: detectors reference these constants instead
// of bare literals, and plan's registry-completeness test iterates Signals() to
// prove every one has a remediation template — so a new detector cannot be silently
// dropped by plan for lack of an entry. Adding a detector means adding its constant
// here and to Signals() below.
const (
	SignalStaleFile      = "stale_file"
	SignalDeadModule     = "dead_module"
	SignalGiantFile      = "giant_file"
	SignalLongFunction   = "long_function"
	SignalDeepNesting    = "deep_nesting"
	SignalVagueName      = "vague_name"
	SignalTodoMarker     = "todo_marker"
	SignalBroadCatch     = "broad_catch"
	SignalNoisyComment   = "noisy_comment"
	SignalSwallowedError = "swallowed_error"
)

// Signals returns every signal a detector in this package can emit.
func Signals() []string {
	return []string{
		SignalStaleFile,
		SignalDeadModule,
		SignalGiantFile,
		SignalLongFunction,
		SignalDeepNesting,
		SignalVagueName,
		SignalTodoMarker,
		SignalBroadCatch,
		SignalNoisyComment,
		SignalSwallowedError,
	}
}
