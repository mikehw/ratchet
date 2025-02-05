package classifier_worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/dynoinc/ratchet/internal"
	"github.com/dynoinc/ratchet/internal/background"
	"github.com/dynoinc/ratchet/internal/llm"
	"github.com/dynoinc/ratchet/internal/storage/schema"
	"github.com/dynoinc/ratchet/internal/storage/schema/dto"
)

type Config struct {
	IncidentClassificationBinary string `split_words:"true" required:"true"`
}

type classifierWorker struct {
	river.WorkerDefaults[background.ClassifierArgs]

	incidentBinary string
	bot            *internal.Bot
	llmClient      *llm.Client
}

func New(c Config, bot *internal.Bot, llmClient *llm.Client) (river.Worker[background.ClassifierArgs], error) {
	if _, err := exec.LookPath(c.IncidentClassificationBinary); err != nil {
		return nil, fmt.Errorf("looking up incident classification binary: %w", err)
	}

	return &classifierWorker{
		incidentBinary: c.IncidentClassificationBinary,
		bot:            bot,
		llmClient:      llmClient,
	}, nil
}

func (w *classifierWorker) Work(ctx context.Context, job *river.Job[background.ClassifierArgs]) error {
	msg, err := w.bot.GetMessage(ctx, job.Args.ChannelID, job.Args.SlackTS)
	if err != nil {
		if errors.Is(err, internal.ErrMessageNotFound) {
			slog.WarnContext(ctx, "message not found", "channel_id", job.Args.ChannelID, "slack_ts", job.Args.SlackTS)
			return nil
		}

		return fmt.Errorf("getting message: %w", err)
	}

	action, err := runIncidentBinary(w.incidentBinary, msg.Message.BotUsername, msg.Message.Text)
	if err != nil {
		return fmt.Errorf("classifying incident with binary: %w", err)
	}

	params := schema.UpdateMessageAttrsParams{
		ChannelID: job.Args.ChannelID,
		Ts:        job.Args.SlackTS,
	}
	if action.Action != dto.ActionNone {
		params.Attrs = dto.MessageAttrs{IncidentAction: action}
	} else {
		services, err := schema.New(w.bot.DB).GetServices(ctx)
		if err != nil {
			return fmt.Errorf("getting services: %w", err)
		}

		// Use llm to classify which service the message belongs to
		service, err := w.llmClient.ClassifyService(ctx, msg.Message.Text, services)
		if err != nil {
			return fmt.Errorf("classifying service: %w", err)
		}

		if service == "" {
			return nil
		}

		params.Attrs = dto.MessageAttrs{AIClassification: dto.AIClassification{Service: service}}
	}

	slog.DebugContext(ctx, "classified message", "channel_id", params.ChannelID, "slack_ts", params.Ts, "text", msg.Message.Text, "attrs", params.Attrs)

	tx, err := w.bot.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := schema.New(w.bot.DB).WithTx(tx)
	if err := qtx.UpdateMessageAttrs(ctx, params); err != nil {
		return fmt.Errorf("updating message %s (ts=%s): %w", params.ChannelID, params.Ts, err)
	}

	if params.Attrs.IncidentAction.Action == dto.ActionOpenIncident {
		riverclient := river.ClientFromContext[pgx.Tx](ctx)

		if !job.Args.IsBackfill {
			// Only post runbook for new messages.
			if _, err := riverclient.InsertTx(ctx, tx, background.PostRunbookWorkerArgs{
				ChannelID: params.ChannelID,
				SlackTS:   params.Ts,
			}, nil); err != nil {
				return fmt.Errorf("scheduling runbook worker: %w", err)
			}
		}

		// schedule a job to update runbook 1 day after the incident is opened
		ts, err := internal.TsToTime(params.Ts)
		if err != nil {
			return fmt.Errorf("converting slack ts (%s) to time: %w", params.Ts, err)
		}

		if _, err := riverclient.InsertTx(ctx, tx, background.UpdateRunbookWorkerArgs{
			ChannelID: params.ChannelID,
			SlackTS:   params.Ts,
		}, &river.InsertOpts{
			Queue:       "update_runbook",
			ScheduledAt: ts.Add(24 * time.Hour),
			Priority:    4, // update runbook as late as possible
		}); err != nil {
			return fmt.Errorf("scheduling runbook worker: %w", err)
		}
	}

	if _, err = river.JobCompleteTx[*riverpgxv5.Driver](ctx, tx, job); err != nil {
		return fmt.Errorf("completing job: %w", err)
	}

	return tx.Commit(ctx)
}

type binaryInput struct {
	Username string `json:"username"`
	Text     string `json:"text"`
}

func runIncidentBinary(binaryPath string, username, text string) (dto.IncidentAction, error) {
	input := binaryInput{
		Username: username,
		Text:     text,
	}

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return dto.IncidentAction{}, fmt.Errorf("marshaling input: %w", err)
	}

	var stdout bytes.Buffer
	cmd := exec.Command(binaryPath)
	cmd.Stdin = bytes.NewReader(inputJSON)
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return dto.IncidentAction{}, fmt.Errorf("running incident classification binary %s: %w", binaryPath, err)
	}

	var output dto.IncidentAction
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return dto.IncidentAction{}, fmt.Errorf("parsing output from binary: %w", err)
	}

	return output, nil
}
