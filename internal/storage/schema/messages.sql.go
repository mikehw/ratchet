// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.28.0
// source: messages.sql

package schema

import (
	"context"

	dto "github.com/dynoinc/ratchet/internal/storage/schema/dto"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

const addMessage = `-- name: AddMessage :exec
INSERT INTO
    messages_v2 (channel_id, ts, attrs)
VALUES
    ($1, $2, $3) ON CONFLICT (channel_id, ts) DO NOTHING
`

type AddMessageParams struct {
	ChannelID string
	Ts        string
	Attrs     dto.MessageAttrs
}

func (q *Queries) AddMessage(ctx context.Context, arg AddMessageParams) error {
	_, err := q.db.Exec(ctx, addMessage, arg.ChannelID, arg.Ts, arg.Attrs)
	return err
}

const deleteOldMessages = `-- name: DeleteOldMessages :exec
DELETE FROM
    messages_v2
WHERE
    channel_id = $1
    AND CAST(ts AS numeric) < EXTRACT(
        epoch
        FROM
            NOW() - $2 :: interval
    )
`

type DeleteOldMessagesParams struct {
	ChannelID string
	OlderThan pgtype.Interval
}

func (q *Queries) DeleteOldMessages(ctx context.Context, arg DeleteOldMessagesParams) error {
	_, err := q.db.Exec(ctx, deleteOldMessages, arg.ChannelID, arg.OlderThan)
	return err
}

const getAlerts = `-- name: GetAlerts :many
SELECT
    service :: text,
    alert :: text,
    priority :: text
FROM
    (
        SELECT
            DISTINCT attrs -> 'incident_action' ->> 'service' as service,
            attrs -> 'incident_action' ->> 'alert' as alert,
            attrs -> 'incident_action' ->> 'priority' as priority
        FROM
            messages_v2
        WHERE
            (
                $1 :: text = '*'
                OR attrs -> 'incident_action' ->> 'service' = $1 :: text
            )
            AND attrs -> 'incident_action' ->> 'action' = 'open_incident'
    ) subq
`

type GetAlertsRow struct {
	Service  string
	Alert    string
	Priority string
}

func (q *Queries) GetAlerts(ctx context.Context, service string) ([]GetAlertsRow, error) {
	rows, err := q.db.Query(ctx, getAlerts, service)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetAlertsRow
	for rows.Next() {
		var i GetAlertsRow
		if err := rows.Scan(&i.Service, &i.Alert, &i.Priority); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getAllMessages = `-- name: GetAllMessages :many
SELECT
    channel_id,
    ts,
    attrs
FROM
    messages_v2
WHERE
    channel_id = $1
`

type GetAllMessagesRow struct {
	ChannelID string
	Ts        string
	Attrs     dto.MessageAttrs
}

func (q *Queries) GetAllMessages(ctx context.Context, channelID string) ([]GetAllMessagesRow, error) {
	rows, err := q.db.Query(ctx, getAllMessages, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetAllMessagesRow
	for rows.Next() {
		var i GetAllMessagesRow
		if err := rows.Scan(&i.ChannelID, &i.Ts, &i.Attrs); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getAllOpenIncidentMessages = `-- name: GetAllOpenIncidentMessages :many
SELECT
    channel_id,
    ts,
    attrs
FROM
    messages_v2
WHERE
    channel_id = $1
    AND attrs -> 'incident_action' ->> 'action' = 'open_incident'
    AND attrs -> 'incident_action' ->> 'service' = $2 :: text
    AND attrs -> 'incident_action' ->> 'alert' = $3 :: text
ORDER BY
    CAST(ts AS numeric) ASC
`

type GetAllOpenIncidentMessagesParams struct {
	ChannelID string
	Service   string
	Alert     string
}

type GetAllOpenIncidentMessagesRow struct {
	ChannelID string
	Ts        string
	Attrs     dto.MessageAttrs
}

func (q *Queries) GetAllOpenIncidentMessages(ctx context.Context, arg GetAllOpenIncidentMessagesParams) ([]GetAllOpenIncidentMessagesRow, error) {
	rows, err := q.db.Query(ctx, getAllOpenIncidentMessages, arg.ChannelID, arg.Service, arg.Alert)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetAllOpenIncidentMessagesRow
	for rows.Next() {
		var i GetAllOpenIncidentMessagesRow
		if err := rows.Scan(&i.ChannelID, &i.Ts, &i.Attrs); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getLatestServiceUpdates = `-- name: GetLatestServiceUpdates :many
WITH valid_messages AS (
    SELECT
        channel_id,
        ts,
        attrs,
        embedding
    FROM
        messages_v2
    WHERE
        CAST(ts AS numeric) > EXTRACT(
            epoch
            FROM
                NOW() - $1 :: interval
        )
        AND attrs -> 'message' ->> 'bot_id' != $2 :: text
        AND attrs -> 'incident_action' ->> 'action' IS NULL 
),
semantic_matches AS (
    SELECT
        channel_id,
        ts,
        ROW_NUMBER() OVER (
            ORDER BY
                embedding <=> $3
        ) as semantic_rank
    FROM
        valid_messages
),
lexical_matches AS (
    SELECT
        channel_id,
        ts,
        ROW_NUMBER() OVER (
            ORDER BY
                ts_rank_cd(to_tsvector('english', attrs -> 'message' ->> 'text'), 
                          plainto_tsquery('english', $4 :: text)) DESC
        ) as lexical_rank
    FROM
        valid_messages
),
combined_scores AS (
    SELECT
        s.channel_id :: text as channel_id,
        s.ts :: text as ts,
        0.4 / (60.0 + COALESCE(s.semantic_rank, 1000)) + 0.6 / (60.0 + COALESCE(l.lexical_rank, 1000)) as rrf_score
    FROM
        semantic_matches s FULL
        OUTER JOIN lexical_matches l ON s.channel_id = l.channel_id AND s.ts = l.ts
),
top_matches AS (
    SELECT
        channel_id,
        ts
    FROM
        combined_scores
    ORDER BY
        rrf_score DESC
    LIMIT
        10
)
SELECT
    m.channel_id,
    m.ts,
    m.attrs
FROM
    valid_messages m
    INNER JOIN top_matches t ON m.channel_id = t.channel_id
    AND m.ts = t.ts
`

type GetLatestServiceUpdatesParams struct {
	Interval       pgtype.Interval
	BotID          string
	QueryEmbedding *pgvector.Vector
	QueryText      string
}

type GetLatestServiceUpdatesRow struct {
	ChannelID string
	Ts        string
	Attrs     []byte
}

func (q *Queries) GetLatestServiceUpdates(ctx context.Context, arg GetLatestServiceUpdatesParams) ([]GetLatestServiceUpdatesRow, error) {
	rows, err := q.db.Query(ctx, getLatestServiceUpdates,
		arg.Interval,
		arg.BotID,
		arg.QueryEmbedding,
		arg.QueryText,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetLatestServiceUpdatesRow
	for rows.Next() {
		var i GetLatestServiceUpdatesRow
		if err := rows.Scan(&i.ChannelID, &i.Ts, &i.Attrs); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getMessage = `-- name: GetMessage :one
SELECT
    channel_id,
    ts,
    attrs,
    embedding
FROM
    messages_v2
WHERE
    channel_id = $1
    AND ts = $2
`

type GetMessageParams struct {
	ChannelID string
	Ts        string
}

func (q *Queries) GetMessage(ctx context.Context, arg GetMessageParams) (MessagesV2, error) {
	row := q.db.QueryRow(ctx, getMessage, arg.ChannelID, arg.Ts)
	var i MessagesV2
	err := row.Scan(
		&i.ChannelID,
		&i.Ts,
		&i.Attrs,
		&i.Embedding,
	)
	return i, err
}

const getMessagesWithinTS = `-- name: GetMessagesWithinTS :many
SELECT
    channel_id,
    ts,
    attrs
FROM
    messages_v2
WHERE
    channel_id = $1
    AND ts BETWEEN $2
    AND $3
`

type GetMessagesWithinTSParams struct {
	ChannelID string
	StartTs   string
	EndTs     string
}

type GetMessagesWithinTSRow struct {
	ChannelID string
	Ts        string
	Attrs     dto.MessageAttrs
}

func (q *Queries) GetMessagesWithinTS(ctx context.Context, arg GetMessagesWithinTSParams) ([]GetMessagesWithinTSRow, error) {
	rows, err := q.db.Query(ctx, getMessagesWithinTS, arg.ChannelID, arg.StartTs, arg.EndTs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []GetMessagesWithinTSRow
	for rows.Next() {
		var i GetMessagesWithinTSRow
		if err := rows.Scan(&i.ChannelID, &i.Ts, &i.Attrs); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const getServices = `-- name: GetServices :many
SELECT
    service :: text
FROM
    (
        SELECT
            DISTINCT attrs -> 'incident_action' ->> 'service' as service
        FROM
            messages_v2
        WHERE
            attrs -> 'incident_action' ->> 'service' IS NOT NULL
    ) s
`

func (q *Queries) GetServices(ctx context.Context) ([]string, error) {
	rows, err := q.db.Query(ctx, getServices)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []string
	for rows.Next() {
		var service string
		if err := rows.Scan(&service); err != nil {
			return nil, err
		}
		items = append(items, service)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

const updateMessageAttrs = `-- name: UpdateMessageAttrs :exec
UPDATE
    messages_v2
SET
    attrs = COALESCE(attrs, '{}' :: jsonb) || $1,
    embedding = $2
WHERE
    channel_id = $3
    AND ts = $4
`

type UpdateMessageAttrsParams struct {
	Attrs     dto.MessageAttrs
	Embedding *pgvector.Vector
	ChannelID string
	Ts        string
}

func (q *Queries) UpdateMessageAttrs(ctx context.Context, arg UpdateMessageAttrsParams) error {
	_, err := q.db.Exec(ctx, updateMessageAttrs,
		arg.Attrs,
		arg.Embedding,
		arg.ChannelID,
		arg.Ts,
	)
	return err
}
