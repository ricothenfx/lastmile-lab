package llm

import "context"

// Noop provider tanpa API key: selalu ErrNoLLM. Tidak ada network call,
// tidak ada state — fitur copilot tersembunyi dan aplikasi tetap utuh.
type Noop struct{}

func (Noop) Complete(context.Context, Request) (*Response, error) {
	return nil, ErrNoLLM
}
