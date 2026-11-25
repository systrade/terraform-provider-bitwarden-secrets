package provider

import "context"

// requestGate serializes SDK network calls for one configured provider instance.
// All resources and data sources must share the gate created during Configure.
type requestGate chan struct{}

// Do waits for the gate before calling fn. It cannot interrupt an SDK call that
// has already started, since the SDK does not accept a context.
func (g requestGate) Do(ctx context.Context, fn func() error) error {
	select {
	case g <- struct{}{}:
		defer func() { <-g }()
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return fn()
}
