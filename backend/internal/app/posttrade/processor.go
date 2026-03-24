package posttrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	readmodel "github.com/xiaobao/rgperp/backend/internal/domain/readmodel"
	riskdomain "github.com/xiaobao/rgperp/backend/internal/domain/risk"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

const recalculateRequestedEventType = "risk.recalculate.requested"

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID(prefix string) string
}

type Processor struct {
	clock  Clock
	idgen  IDGenerator
	outbox riskdomain.OutboxPublisher
}

func NewProcessor(clock Clock, idgen IDGenerator, outbox riskdomain.OutboxPublisher) *Processor {
	if clock == nil || idgen == nil || outbox == nil {
		return nil
	}
	return &Processor{
		clock:  clock,
		idgen:  idgen,
		outbox: outbox,
	}
}

func (p *Processor) RecalculateAfterTrade(ctx context.Context, userID uint64, traceID string) error {
	_, err := p.QueueRecalculation(ctx, userID, "trade_fill", traceID)
	return err
}

func (p *Processor) RecalculateAccount(ctx context.Context, userID uint64, operatorID string) (readmodel.AdminRiskRecalculationResult, error) {
	requestID, err := p.QueueRecalculation(ctx, userID, "admin", "admin:"+strings.TrimSpace(operatorID))
	if err != nil {
		return readmodel.AdminRiskRecalculationResult{}, err
	}
	return readmodel.AdminRiskRecalculationResult{
		UserID:                 userID,
		TriggeredBy:            "admin",
		RecalculationRequestID: &requestID,
		RecalculationStatus:    stringPtr("QUEUED"),
	}, nil
}

func (p *Processor) QueueRecalculation(ctx context.Context, userID uint64, triggeredBy string, traceID string) (string, error) {
	if p == nil || p.clock == nil || p.idgen == nil || p.outbox == nil {
		return "", nil
	}
	if userID == 0 {
		return "", fmt.Errorf("%w: user id is required", errorsx.ErrInvalidArgument)
	}
	triggeredBy = strings.TrimSpace(triggeredBy)
	if triggeredBy == "" {
		triggeredBy = "manual"
	}
	requestID := p.idgen.NewID("rrq")
	if err := p.outbox.Publish(ctx, riskdomain.DomainEvent{
		EventID:       p.idgen.NewID("evt"),
		AggregateType: "risk_recalculation",
		AggregateID:   requestID,
		EventType:     recalculateRequestedEventType,
		Payload: map[string]any{
			"request_id":   requestID,
			"user_id":      userID,
			"triggered_by": triggeredBy,
			"trace_id":     strings.TrimSpace(traceID),
		},
		CreatedAt: p.clock.Now().UTC(),
	}); err != nil {
		return "", err
	}
	return requestID, nil
}

func stringPtr(value string) *string {
	return &value
}
