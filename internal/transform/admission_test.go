package transform

import (
	"context"
	"errors"
	"testing"
)

func TestAdmission(t *testing.T) {
	g := newRequestGate(1)
	if err := g.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := g.acquire(context.Background()); !errors.Is(err, errBusy) {
		t.Fatalf("overlap: %v", err)
	}
	g.release()
	if err := g.acquire(context.Background()); err != nil {
		t.Fatalf("capacity was not returned: %v", err)
	}
	g.release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled request: %v", err)
	}
}
