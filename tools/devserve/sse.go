// SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"strings"
	"sync"
)

// sseBroker fans server-sent events out to every open /events connection.
type sseBroker struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newSSEBroker() *sseBroker {
	return &sseBroker{clients: make(map[chan string]struct{})}
}

// subscribe registers a client; the channel is buffered so a slow client cannot
// block broadcast.
func (b *sseBroker) subscribe() chan string {
	ch := make(chan string, 8)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *sseBroker) unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

// broadcast sends msg to every subscriber, dropping it for any whose buffer is full.
func (b *sseBroker) broadcast(msg string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// formatSSE renders one named event; each data line gets its own "data:" field
// so multi-line compiler output survives.
func formatSSE(event, data string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\n", event)
	if data == "" {
		b.WriteString("data: \n")
	} else {
		for _, line := range strings.Split(data, "\n") {
			fmt.Fprintf(&b, "data: %s\n", line)
		}
	}
	b.WriteString("\n")
	return b.String()
}
