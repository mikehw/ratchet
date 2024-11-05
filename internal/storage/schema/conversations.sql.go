// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0
// source: conversations.sql

package schema

import (
	"context"

	dto "github.com/dynoinc/ratchet/internal/storage/schema/dto"
)

const addMessage = `-- name: AddMessage :exec
INSERT INTO messages (channel_id, slack_ts, attrs)
VALUES ($1, $2, $3)
ON CONFLICT (channel_id, slack_ts) DO NOTHING
`

type AddMessageParams struct {
	ChannelID string
	SlackTs   string
	Attrs     dto.MessageAttrs
}

func (q *Queries) AddMessage(ctx context.Context, arg AddMessageParams) error {
	_, err := q.db.Exec(ctx, addMessage, arg.ChannelID, arg.SlackTs, arg.Attrs)
	return err
}

const getMessage = `-- name: GetMessage :one
SELECT channel_id, slack_ts, attrs FROM messages WHERE channel_id = $1 AND slack_ts = $2
`

type GetMessageParams struct {
	ChannelID string
	SlackTs   string
}

func (q *Queries) GetMessage(ctx context.Context, arg GetMessageParams) (Message, error) {
	row := q.db.QueryRow(ctx, getMessage, arg.ChannelID, arg.SlackTs)
	var i Message
	err := row.Scan(&i.ChannelID, &i.SlackTs, &i.Attrs)
	return i, err
}
