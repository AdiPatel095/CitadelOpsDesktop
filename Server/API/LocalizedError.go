package API

import (
	"CitadelDesktop/Server/Localization"
	"net/http"
)

func localizedError(code, message string, descriptors []*Localization.Message) map[string]any {
	var descriptor *Localization.Message
	if len(descriptors) > 0 {
		descriptor = Localization.Bind(descriptors[0], message)
	}
	return map[string]any{"code": code, "message": message, "messageDescriptor": descriptor, "translationStatus": Localization.Status(descriptor)}
}

func writeErrorFromError(writer http.ResponseWriter, status int, code string, err error) {
	writeError(writer, status, code, err.Error(), Localization.FromError(err))
}
func errorEnvelopeFromError(id, code string, err error) Envelope {
	return errorEnvelope(id, code, err.Error(), Localization.FromError(err))
}
