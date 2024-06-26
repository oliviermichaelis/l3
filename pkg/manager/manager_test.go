package manager

import (
	"context"
	"testing"
	"time"

	"github.com/oliviermichaelis/l3/pkg/entry"
)

func TestManager_EnqueuePeriodically(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		description     string
		entries         map[string]*entry.Entry
		expectedEntries int
	}{
		{
			description: "return error",
			entries: map[string]*entry.Entry{
				"service-1": {},
				"service-2": {},
			},
			expectedEntries: 10,
		},
	}

	for _, tc := range testcases {
		tc := tc // otherwise the underlying value will change
		t.Run(tc.description, func(t *testing.T) {
			ch := make(chan<- *entry.Entry, tc.expectedEntries)
			m := Manager{
				entries:      tc.entries,
				updatePeriod: time.Millisecond * 1,
			}

			ctx, cancelCtx := context.WithCancel(context.Background())
			go m.enqueuePeriodically(ctx, ch)

			// This is super hacky to spare me from mocking an own ticker
			time.Sleep(500 * time.Millisecond)
			cancelCtx()

			if len(ch) != tc.expectedEntries {
				t.Errorf("there should have been %d entries in the channel", tc.expectedEntries)
			}
		})
	}
}
