package State

// RetainProtocolObservation reports whether an opcode has a current state
// consumer. All wire frames still reach exact-commit watchers and telemetry;
// only this bounded set becomes durable GameState.
func RetainProtocolObservation(opcode string) bool {
	switch opcode {
	case "gbd": // authenticated baseline readiness
		return true
	case "ain": // alliance station command ordering
		return true
	case "crin": // crafting snapshot freshness
		return true
	case "gei", "ggm": // equipment and gem sale freshness guards
		return true
	default:
		return false
	}
}

// RetainOutboundProtocolObservation reports whether application logic needs a
// durable outbound ordering marker. Other retained protocols use only their
// successful inbound freshness; the complete outbound wire remains available
// in telemetry without creating an otherwise empty state revision.
func RetainOutboundProtocolObservation(opcode string) bool {
	return opcode == "ain"
}
