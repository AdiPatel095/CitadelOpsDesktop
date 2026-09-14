package Outbound

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestRouterPausesCDSForEveryCallerBeforeTransport(t *testing.T) {
	for _, actor := range []string{"", "api", "automation:autoBird", "automation:autoStation", "automation:autoStorm"} {
		t.Run(actor, func(t *testing.T) {
			var sent []string
			router := NewRouter(context.Background(), Config{
				Ready: func() bool { return true },
				Send: func(_ context.Context, payload []byte) error {
					frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now())
					if err != nil {
						return err
					}
					sent = append(sent, frame.Opcode)
					return nil
				},
			})
			defer router.Close()
			ctx, cancel := context.WithTimeout(WithMetadata(context.Background(), Metadata{Actor: actor}), time.Second)
			defer cancel()
			for _, opcode := range []string{"cds", "CDS"} {
				payload := []byte("%xt%EmpireEx_21%" + opcode + "%1%{\"SID\":123,\"A\":[[215,10]]}%")
				if err := router.Send(ctx, payload); !errors.Is(err, ErrCDSPaused) {
					t.Fatalf("%s error = %v; want CDS pause", opcode, err)
				}
			}
			if len(sent) != 0 {
				t.Fatalf("blocked CDS reached transport: %v", sent)
			}
			for _, opcode := range []string{"gam", "cra"} {
				if err := router.Send(ctx, outboundTestPayload(t, opcode, "allowed")); err != nil {
					t.Fatalf("unrelated %s command failed: %v", opcode, err)
				}
			}
			if !reflect.DeepEqual(sent, []string{"gam", "cra"}) {
				t.Fatalf("transport sends = %v", sent)
			}
		})
	}
}
