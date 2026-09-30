package Intent

// estimatedReceiptBytes is a deterministic, allocation-free size estimate used for
// the in-memory receipt budget. It is not exact.
func estimatedReceiptBytes(receipt Receipt) int {
	size := 1024 + len(receipt.ID) + len(receipt.Intent) + len(receipt.Actor) + len(receipt.Error) + len(receipt.RawError)
	if receipt.Plan != nil {
		size += 512 + len(receipt.Plan.Summary) + estimatedStepBytes(receipt.Plan.Steps)
	}
	for _, exchange := range receipt.Exchanges {
		size += 256 + len(exchange.Command.Payload)
		if exchange.Response != nil {
			size += 256 + len(exchange.Response.Payload) + len(exchange.Response.PayloadText) + len(exchange.Response.Raw)
		}
	}
	for _, evidence := range receipt.Evidence {
		size += 64 + len(evidence.Kind) + len(evidence.Data)
	}
	if receipt.Failure != nil {
		size += 512
	}
	return size
}

func estimatedStepBytes(steps []Step) int {
	size := 0
	for _, step := range steps {
		size += 256 + len(step.Payload) + len(step.ActionArguments) + len(step.ResolverArguments) +
			len(step.ExpectedResponsePayload) + len(step.PreDispatchArguments) + len(step.FinalDispatchArguments) +
			len(step.DefinitiveSendFailureArguments) + len(step.DefinitiveResponseFailureArguments) + len(step.Command.Payload)
		size += estimatedStepBytes(step.Batch)
	}
	return size
}
