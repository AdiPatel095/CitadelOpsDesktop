package Session

import (
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"testing"
	"time"
)

func queueTestFrame(raw, payload string) queuedIngestFrame {
	observed := Ingest.ObservedFrame{Frame: Protocol.Frame{Raw: raw, Payload: []byte(payload)}}
	return queuedIngestFrame{observed: observed, bytes: len(raw) + len(payload)}
}

func TestIngestQueueBoundsBytesAndAdmitsOneOversizedFrame(t *testing.T) {
	if ingestQueueFrameLimit != 8192 || ingestQueueByteLimit != 32<<20 {
		t.Fatal("queue defaults changed")
	}
	queue := newIngestFrameQueue(2, 10)
	first, second := queueTestFrame("abc", "de"), queueTestFrame("fgh", "ij")
	if !queue.push(first) || !queue.push(second) || queue.bytes != 10 {
		t.Fatal("exact byte budget did not fit")
	}
	if got, ok := queue.pop(); !ok || got.observed.Frame.Raw != "abc" || queue.bytes != 5 {
		t.Fatal("pop accounting/order incorrect")
	}
	if got, ok := queue.pop(); !ok || got.observed.Frame.Raw != "fgh" || queue.bytes != 0 {
		t.Fatal("second pop accounting/order incorrect")
	}
	oversized := queueTestFrame("01234567890", "payload")
	if !queue.push(oversized) || queue.bytes != 18 {
		t.Fatal("empty queue rejected oversized frame")
	}
	pending := queue.closeAndTakePending()
	if len(pending) != 1 || pending[0].bytes != 18 || queue.bytes != 0 || len(queue.frames) != 0 {
		t.Fatal("close did not return/reset pending frames")
	}
	if queue.push(first) {
		t.Fatal("closed queue accepted frame")
	}
	if _, ok := queue.pop(); ok {
		t.Fatal("closed empty queue yielded frame")
	}
	if len(queue.closeAndTakePending()) != 0 {
		t.Fatal("close returned frames twice")
	}
}

func TestIngestQueueBlocksProducerUntilSpaceFrees(t *testing.T) {
	for _, tc := range []struct {
		name            string
		capacity, limit int
		first, second   queuedIngestFrame
	}{
		{"bytes", 4, 10, queueTestFrame("123456", ""), queueTestFrame("abcdef", "g")},
		{"frame count", 1, 100, queueTestFrame("a", ""), queueTestFrame("b", "")},
		{"oversized head", 4, 10, queueTestFrame("01234567890", ""), queueTestFrame("b", "")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			queue := newIngestFrameQueue(tc.capacity, tc.limit)
			queue.push(tc.first)
			entered := make(chan struct{})
			done := make(chan bool, 1)
			go func() { close(entered); done <- queue.push(tc.second) }()
			<-entered
			select {
			case <-done:
				t.Fatal("producer exceeded queue bounds")
			case <-time.After(20 * time.Millisecond):
			}
			first, ok := queue.pop()
			if !ok || first.observed.Frame.Raw != tc.first.observed.Frame.Raw {
				t.Fatal("FIFO head changed")
			}
			select {
			case accepted := <-done:
				if !accepted {
					t.Fatal("producer rejected after space freed")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("producer did not wake")
			}
			if queue.bytes != tc.second.bytes {
				t.Fatal("byte accounting lost blocked producer")
			}
			pending := queue.closeAndTakePending()
			if len(pending) != 1 || pending[0].observed.Frame.Raw != tc.second.observed.Frame.Raw {
				t.Fatal("pending order changed")
			}
		})
	}
	t.Run("close releases blocked producer", func(t *testing.T) {
		queue := newIngestFrameQueue(1, 10)
		queue.push(queueTestFrame("head", ""))
		done := make(chan bool, 1)
		go func() { done <- queue.push(queueTestFrame("next", "")) }()
		queue.closeAndTakePending()
		select {
		case accepted := <-done:
			if accepted {
				t.Fatal("closed queue accepted blocked producer")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("close did not wake producer")
		}
	})
}
