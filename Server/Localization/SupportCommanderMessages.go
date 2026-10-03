package Localization

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Pending English stays in the story file; generated locale catalogs are untouched.
//
//go:embed pending/CIT-133.json
var supportCommanderMessagesJSON []byte
var supportCommanderMessages = func() map[string]string {
	var document struct {
		Messages map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(supportCommanderMessagesJSON, &document); err != nil {
		panic(err)
	}
	return document.Messages
}()

func SupportCommanderMessage(key string, params Params) *Message {
	template, ok := supportCommanderMessages[key]
	if !ok {
		panic(key)
	}
	text := template
	for name, value := range params {
		text = strings.ReplaceAll(text, "{"+name+"}", fmt.Sprint(value))
	}
	return Bind(New(key, template, params), text)
}
