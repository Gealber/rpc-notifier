package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Notifier send a notification throw a telegram bot
// Using sendMessage method.
// https://core.telegram.org/bots/api#sendmessage

type TGMessage struct {
	// ChatID can be an integer of a string
	ChatID any    `json:"chat_id"`
	Text   string `json:"text"`
}

type Notifier struct {
	botAPIKey string
	subs      []string
}

func NewNotifier() *Notifier {
	// notifiers separated by comma
	subsStr := os.Getenv("NOTIFIER_SUBS")
	subs := strings.Split(subsStr, ",")

	return &Notifier{
		botAPIKey: os.Getenv("TGBOT_API_KEY"),
		subs:      subs,
	}
}

func (svc *Notifier) Notify(text string) error {
	for _, sub := range svc.subs {
		msg := TGMessage{
			ChatID: sub,
			Text:   text,
		}

		b, err := json.Marshal(&msg)
		if err != nil {
			return err
		}

		body := bytes.NewBuffer(b)

		url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", svc.botAPIKey)
		_, err = http.Post(url, "application/json", body)
		if err != nil {
			return err
		}
	}

	return nil
}
