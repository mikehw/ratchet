package dto

import (
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
)

type MessageAttrs struct {
	Upstream *slackevents.MessageEvent `json:"upstream,omitempty"` // from live update
	Message  *slack.Message            `json:"message,omitempty"`  // from history

	// Set in case this is a incident open/close message
	IncidentID int32  `json:"incident_id,omitempty"`
	Action     string `json:"action,omitempty"`

	// Set in case this is a bot message
	BotName string `json:"bot_name,omitempty"`

	// Set in case this is a message from a user
	UserID string `json:"user_id,omitempty"`
}

type IncidentAttrs struct {
}
