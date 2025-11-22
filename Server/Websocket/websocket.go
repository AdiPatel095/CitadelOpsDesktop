package Websocket

import (
	"context"
	"encoding/base64"
	"log"
	"sync"
	"unicode/utf8"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// IncomingMessages is a channel for incoming WebSocket messages from the server.
var IncomingMessages = make(chan []byte, 100)

// OutgoingMessages is a channel for outgoing WebSocket messages sent by the browser.
var OutgoingMessages = make(chan []byte, 100)

// SetupWebSocketListener sets up the chromedp event listener for WebSocket traffic.
// It funnels incoming and outgoing messages into the package-level channels.
func SetupWebSocketListener(ctx context.Context, wsURL string) {
	var interestingRequestIDs sync.Map

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch e := ev.(type) {

		case *network.EventWebSocketCreated:
			if e.URL == wsURL {
				log.Printf("[WS CONNECTED] ID: %s | URL: %s", e.RequestID, e.URL)
				interestingRequestIDs.Store(e.RequestID, true)
			}

		case *network.EventWebSocketFrameSent:
			if _, ok := interestingRequestIDs.Load(e.RequestID); ok {
				if payloadBytes, ok := handleWebSocketFrame(e.Response, "SENT", ">>>"); ok {
					// Send to channel for application consumption
					if utf8.Valid(payloadBytes) {
						OutgoingMessages <- payloadBytes
					}
				}
			}

		case *network.EventWebSocketFrameReceived:
			if _, ok := interestingRequestIDs.Load(e.RequestID); ok {
				if payloadBytes, ok := handleWebSocketFrame(e.Response, "RECV", "<<<"); ok {
					// Send to channel for application consumption
					if utf8.Valid(payloadBytes) {
						IncomingMessages <- payloadBytes
					}
				}
			}

		case *network.EventWebSocketClosed:
			if _, ok := interestingRequestIDs.Load(e.RequestID); ok {
				log.Printf("[WS CLOSED] ID: %s", e.RequestID)
				interestingRequestIDs.Delete(e.RequestID)
			}
		}
	})
}

// handleWebSocketFrame processes a WebSocket frame, decodes its payload, and logs it.
// It returns the payload as a byte slice and a boolean indicating success.
func handleWebSocketFrame(resp *network.WebSocketFrame, direction, prefix string) ([]byte, bool) {
	var payloadBytes []byte
	var err error

	switch resp.Opcode {
	case 1: // Text Frame
		payloadBytes = []byte(resp.PayloadData)
	case 2: // Binary Frame
		payloadBytes, err = base64.StdEncoding.DecodeString(resp.PayloadData)
		if err != nil {
			log.Printf("%s [%s] Error decoding base64 binary payload: %v | Raw: %s", prefix, direction, err, resp.PayloadData)
			return nil, false
		}
	default:
		log.Printf("%s [%s] Unknown opcode type: %d | Raw: %s", prefix, direction, int(resp.Opcode), resp.PayloadData)
		return nil, false
	}

	return payloadBytes, true
}
