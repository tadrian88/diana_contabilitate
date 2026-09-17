package workerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"diana-contabilitate/backend/internal/outbox"

	"github.com/hibiken/asynq"
)

type Publisher interface {
	Publish(context.Context, Job) (string, error)
}

type AsynqPublisher struct {
	client   *asynq.Client
	queue    string
	maxRetry int
	timeout  time.Duration
}

func (p *AsynqPublisher) PublishSPVSync(ctx context.Context, connectionID string) (string, error) {
	payload, err := json.Marshal(SPVSyncJob{ConnectionID: connectionID})
	if err != nil {
		return "", err
	}
	info, err := p.client.EnqueueContext(ctx, asynq.NewTask(SPVSyncTask, payload), asynq.Queue(p.queue), asynq.MaxRetry(p.maxRetry), asynq.Timeout(p.timeout), asynq.Unique(time.Minute))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

func (p *AsynqPublisher) PublishSPVDocument(ctx context.Context, documentID string) (string, error) {
	payload, err := json.Marshal(SPVDocumentJob{DocumentID: documentID})
	if err != nil {
		return "", err
	}
	info, err := p.client.EnqueueContext(ctx, asynq.NewTask(SPVDocumentTask, payload), asynq.Queue(p.queue), asynq.MaxRetry(p.maxRetry), asynq.Timeout(p.timeout))
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

func NewAsynqPublisher(client *asynq.Client, queue string, maxRetry int, timeout time.Duration) *AsynqPublisher {
	return &AsynqPublisher{client: client, queue: queue, maxRetry: maxRetry, timeout: timeout}
}

func (p *AsynqPublisher) Publish(ctx context.Context, job Job) (string, error) {
	payload, err := json.Marshal(job)
	if err != nil {
		return "", err
	}
	taskType := ContinueInvoiceTask
	if job.EventType == outbox.EventContractAvailable {
		taskType = ContractAvailableTask
	} else if job.EventType == outbox.EventContractExtractionRequested {
		taskType = ContractExtractionTask
	} else if job.EventType == outbox.EventContractActivationRequested {
		taskType = ContractActivationTask
	}
	info, err := p.client.EnqueueContext(ctx, asynq.NewTask(taskType, payload), asynq.Queue(p.queue), asynq.MaxRetry(p.maxRetry), asynq.Timeout(p.timeout))
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
