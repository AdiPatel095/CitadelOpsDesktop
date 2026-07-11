package API

import "encoding/json"

const ContractVersion = 2

type Envelope struct {
	Version  int             `json:"v"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type"`
	Revision uint64          `json:"revision,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

func newEnvelope(id string, messageType string, revision uint64, payload any) Envelope {
	raw, _ := json.Marshal(payload)
	return Envelope{Version: ContractVersion, ID: id, Type: messageType, Revision: revision, Payload: raw}
}
