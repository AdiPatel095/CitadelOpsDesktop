package Intent

import (
	"strings"

	"CitadelDesktop/Server/Localization"
)

func StormPackageCapStatus(name string) (string, *Localization.Message) {
	message := Localization.New("server.storm.package_cap_blocked", "Package {name} is at its purchase limit for this event", Localization.Params{"name": name})
	text := strings.ReplaceAll(message.Fallback, "{name}", name)
	return text, Localization.Bind(message, text)
}
