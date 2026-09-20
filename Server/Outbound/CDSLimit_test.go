package Outbound

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
)

func TestRouterAllowsBoundedCDSForEveryCaller(t *testing.T) {
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
				if err := router.Send(ctx, payload); err != nil {
					t.Fatalf("%s error = %v; want bounded CDS accepted", opcode, err)
				}
			}
			if len(sent) != 2 {
				t.Fatalf("bounded CDS did not reach transport: %v", sent)
			}
			for _, opcode := range []string{"gam", "cra"} {
				if err := router.Send(ctx, outboundTestPayload(t, opcode, "allowed")); err != nil {
					t.Fatalf("unrelated %s command failed: %v", opcode, err)
				}
			}
			if !reflect.DeepEqual(sent, []string{"cds", "cds", "gam", "cra"}) {
				t.Fatalf("transport sends = %v", sent)
			}
		})
	}
}

func TestRouterRejectsOversizedRawCDSBeforeTransport(t *testing.T) {
	sends := 0
	router := NewRouter(context.Background(), Config{Ready: func() bool { return true }, Send: func(context.Context, []byte) error { sends++; return nil }})
	defer router.Close()
	for _, count := range []int{11, 20, 26} {
		army := make([][2]int64, count)
		for i := range army {
			army[i] = [2]int64{int64(i + 1), 100}
		}
		body, _ := json.Marshal(map[string]any{"SID": 123, "A": army})
		raw := []byte("%xt%EmpireEx_21%cds%1%" + string(body) + "%")
		if err := router.Send(context.Background(), raw); err == nil {
			t.Fatal("oversized raw CDS accepted")
		}
	}
	if sends != 0 {
		t.Fatal("oversized command reached transport")
	}
}
