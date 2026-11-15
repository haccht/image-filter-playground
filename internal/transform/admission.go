package transform

import (
	"context"
	"errors"
)

// Wasm host backpressure normally prevents overlap before handler entry.
// Reject any overlapping work without buffering bodies or queuing Go tasks.
var admission = newRequestGate(1)

var errBusy = errors.New("request capacity exhausted")

type requestGate struct {
	slots chan struct{}
}

func newRequestGate(capacity int) *requestGate {
	return &requestGate{slots: make(chan struct{}, capacity)}
}

func (g *requestGate) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case g.slots <- struct{}{}:
	default:
		return errBusy
	}
	return nil
}

func (g *requestGate) release() {
	<-g.slots
}
