package cli

import "io"

// Exported for tests: the exit-code mapping is worth covering directly, since
// Execute builds its own signal context and cannot be handed a cancelled one.
const ExitInterrupted = exitInterrupted

func Report(w io.Writer, err error) int { return report(w, err) }
