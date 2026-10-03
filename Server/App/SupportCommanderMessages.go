package App

import (
	"CitadelDesktop/Server/Localization"
	"fmt"
	"strings"
)

// These producer templates populate the English source and its client copy.
// The pending story file lists keys awaiting the locale translation batch.
func supportCommanderMessage(key string, params Localization.Params) *Localization.Message {
	var message *Localization.Message
	switch key {
	case "server.support.commander_wait":
		message = Localization.New("server.support.commander_wait", "Support waiting: no free premium commander and no idle commander", params)
	case "server.support.manual_quota":
		message = Localization.New("server.support.manual_quota", "Not sent: no free premium commanders left today. CitadelOps never spends rubies on commanders.", params)
	case "server.support.manual_vip_unknown":
		message = Localization.New("server.support.manual_vip_unknown", "Not sent: VIP status unavailable. CitadelOps never spends rubies on commanders.", params)
	case "server.support.storm_quota":
		message = Localization.New("server.support.storm_quota", "Waiting: no free premium commanders left today (VIP daily allowance used). Rubies are never spent.", params)
	case "server.support.storm_vip_unknown":
		message = Localization.New("server.support.storm_vip_unknown", "Waiting: VIP status unavailable. Rubies are never spent.", params)
	case "server.support.premium_blocked":
		message = Localization.New("server.support.premium_blocked", "Not sent: free premium commander reservation unavailable. CitadelOps never spends rubies on commanders.", params)
	case "server.rift.premium_capture":
		message = Localization.New("server.rift.premium_capture", "Rift launch {name} was captured with the paid premium commander. Capture it again with one of your own commanders.", params)
	default:
		panic(key)
	}
	text := message.Fallback
	for name, value := range message.Params {
		text = strings.ReplaceAll(text, "{"+name+"}", fmt.Sprint(value))
	}
	return Localization.Bind(message, text)
}
