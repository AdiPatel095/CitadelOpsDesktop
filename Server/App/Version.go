package App

const Version = "2.3.5"

// BuildRevision and BuildID are injected by the release build. Keeping
// explicit local defaults makes development binaries honest about their
// provenance while allowing both desktop and hosted artifacts to prove that
// they came from the same source revision.
var (
	BuildRevision = "development"
	BuildID       = "local"
)
