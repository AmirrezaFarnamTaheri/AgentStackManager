package selfinstall

import "github.com/agentstack/agentstack/internal/pathenv"

// AppendPathSegment is retained as the self-install compatibility surface and
// delegates Windows PATH semantics to the canonical pathenv package.
func AppendPathSegment(current, target string) (string, bool) {
	return pathenv.AppendWindows(current, target)
}
