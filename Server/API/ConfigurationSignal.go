package API

// ConfigurationRevisionSignal is the `config.changed` payload when an external
// authority (the hosted control plane) owns the configuration. The dashboard
// holds the canonical copy already, so the worker sends only which canonical
// version it has applied instead of a full ~60 KB snapshot; the dashboard
// refetches from the authority when that revision is newer than its own.
// External is always true: it is how a dashboard tells this compact signal from
// a full snapshot, and a worker that predates the signal never sets it.
type ConfigurationRevisionSignal struct {
	Revision uint64 `json:"revision"`
	Digest   string `json:"digest"`
	External bool   `json:"external"`
}

// externalConfiguration reports whether an external authority owns portable
// configuration for this worker.
func (server *Server) externalConfiguration() bool {
	return server.config.BackgroundOnly || server.externalConfigurationAuthority.Load()
}

func (server *Server) configurationSignal() ConfigurationRevisionSignal {
	authority := server.config.Configuration.AuthorityVersion()
	return ConfigurationRevisionSignal{Revision: authority.Revision, Digest: authority.Digest, External: true}
}
