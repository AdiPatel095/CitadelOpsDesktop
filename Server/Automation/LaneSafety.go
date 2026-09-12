package Automation

import "CitadelDesktop/Server/Intent"

func receiptLocksLane(receipt Intent.Receipt) bool {
	return receipt.Failure != nil && receipt.Failure.SafetyLock != nil
}
