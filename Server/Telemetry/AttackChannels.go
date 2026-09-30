package Telemetry

// AttackLaunchRetention is how far back confirmed launch counts reach. Any
// source that replaces the telemetry-backed counters keeps the same horizon.
const AttackLaunchRetention = attackLaunchRetention

// FeatureChannelForActor maps an intent actor to its feature channel, or "" for
// an actor with none. It is the single mapping used to attribute a launch to a
// feature, whichever source counts it.
func FeatureChannelForActor(actor string) string {
	return featureChannelForActor(actor)
}

// AttackFeatureChannels lists the channels whose confirmed launches are
// counted, in the same order the attack-rate response reports them.
func AttackFeatureChannels() []string {
	return append([]string(nil), attackFeatureChannels...)
}
