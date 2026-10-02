package App

import (
	"CitadelDesktop/Server/Outbound"
	"context"
	"errors"
)

type countedMapSender struct {
	mapGAASender
	application *Application
	kind        string
}

func (s countedMapSender) Send(ctx context.Context, wire []byte) error {
	err := s.mapGAASender.Send(ctx, wire)
	if !errors.Is(err, Outbound.ErrAutomationLocked) || Outbound.IsIndeterminate(err) {
		s.application.Diagnostics.RecordMapRequest(s.kind)
	}
	return err
}
func (a *Application) countMapSender(kind string) mapGAASender {
	return countedMapSender{a.Session, a, kind}
}
