package API

import "encoding/json"

const ContractVersion = 2

type Envelope struct {
	Version  int             `json:"v"`
	ID       string          `json:"id,omitempty"`
	Type     string          `json:"type"`
	Revision uint64          `json:"revision,omitempty"`
	Sequence uint64          `json:"sequence,omitempty"`
	Gap      bool            `json:"gap,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

func newEnvelope(id string, messageType string, revision uint64, payload any) Envelope {
	raw, _ := json.Marshal(payload)
	return Envelope{Version: ContractVersion, ID: id, Type: messageType, Revision: revision, Payload: raw}
}

func streamEnvelope(id string, messageType string, revision uint64, sequence uint64, gap bool, payload any) Envelope {
	envelope := newEnvelope(id, messageType, revision, payload)
	envelope.Sequence = sequence
	envelope.Gap = gap
	return envelope
}

func streamEnvelopeRaw(
	id string,
	messageType string,
	revision uint64,
	sequence uint64,
	gap bool,
	payload json.RawMessage,
) Envelope {
	return Envelope{
		Version: ContractVersion, ID: id, Type: messageType, Revision: revision,
		Sequence: sequence, Gap: gap, Payload: payload,
	}
}
