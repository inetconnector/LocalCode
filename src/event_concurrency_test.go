// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sync"
	"testing"
)

func TestEventSubscribersConcurrentAddAndUnsubscribe(t *testing.T) {
	state := newRemoteTestState(t)

	const rounds = 250
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			state.AddEvent(UIEvent{Type: "progress", Message: fmt.Sprintf("event-%d", i)})
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < rounds; i++ {
			ch := state.Subscribe()
			state.Unsubscribe(ch)
			// Unsubscribe deliberately does not close producer-visible channels.
			// This assertion would panic immediately if that invariant regressed.
			select {
			case ch <- UIEvent{Type: "test"}:
			default:
			}
		}
	}()

	wg.Wait()
}

func TestEventUnsubscribeIsIdempotent(t *testing.T) {
	state := newRemoteTestState(t)
	ch := state.Subscribe()
	state.Unsubscribe(ch)
	state.Unsubscribe(ch)
	state.Unsubscribe(nil)
}
