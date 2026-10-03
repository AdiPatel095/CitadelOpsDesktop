package App

import (
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
)

type finalDispatchGates struct {
	gates      []Intent.FinalDispatchProvider
	tickets    *travelTicketDispatchGate
	commanders *premiumCommanderDispatchGate
}

func newFinalDispatchGates(coins *coinDispatchGate, tickets *travelTicketDispatchGate, extra ...Intent.FinalDispatchProvider) *finalDispatchGates {
	// Preserve commander, coin and ticket gates; extras include special costs and market barrows.
	commanders := newPremiumCommanderDispatchGate()
	return &finalDispatchGates{gates: append([]Intent.FinalDispatchProvider{commanders, coins, tickets}, extra...), tickets: tickets, commanders: commanders}
}
func (g *finalDispatchGates) Validate(ctx context.Context, input Intent.PlanningContext, step Intent.Step) error {
	for _, gate := range g.gates {
		if err := gate.Validate(ctx, input, step); err != nil {
			// Validation may have reserved on an earlier gate before a later gate blocks.
			g.DefinitiveFailure(ctx, step)
			return err
		}
	}
	return nil
}
func (g *finalDispatchGates) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	for _, gate := range g.gates {
		gate.DefinitiveFailure(ctx, step)
	}
}
func (g *finalDispatchGates) Indeterminate(ctx context.Context, step Intent.Step) {
	for _, gate := range g.gates {
		gate.Indeterminate(ctx, step)
	}
}
func (g *finalDispatchGates) Completed(ctx context.Context, input Intent.PlanningContext, step Intent.Step, response Protocol.CommittedFrame) bool {
	refresh := false
	for _, gate := range g.gates {
		if gate.Completed(ctx, input, step, response) {
			refresh = true
		}
	}
	return refresh
}
func (g *finalDispatchGates) AvailableCurrency(state State.GameState, id State.CurrencyID) (int64, int64, bool) {
	return g.tickets.AvailableCurrency(state, id)
}

func (g *finalDispatchGates) ReservePremiumCommander(input Intent.PlanningContext) (string, bool) {
	return g.commanders.ReservePremiumCommander(input)
}
func (g *finalDispatchGates) SelectSupportCommander(input Intent.PlanningContext, feature string, source State.CastleState) (State.CommanderID, string, error) {
	return g.commanders.SelectSupportCommander(input, feature, source)
}
func (g *finalDispatchGates) ReleaseSupportCommander(token string) {
	g.commanders.ReleaseSupportCommander(token)
}
func (g *finalDispatchGates) OperationFinished(ctx context.Context) {
	g.commanders.OperationFinished(ctx)
}
