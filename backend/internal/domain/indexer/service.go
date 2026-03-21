package indexer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	walletdomain "github.com/xiaobao/rgperp/backend/internal/domain/wallet"
	"github.com/xiaobao/rgperp/backend/internal/pkg/authx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/decimalx"
	"github.com/xiaobao/rgperp/backend/internal/pkg/errorsx"
)

type Service struct {
	wallet    Wallet
	deposits  DepositRepository
	withdraws WithdrawRepository
	addresses DepositAddressResolver
	publisher EventPublisher
	txManager TxManager
	clock     Clock
	idgen     IDGenerator
	chains    map[int64]ChainRule
}

func NewService(
	wallet Wallet,
	deposits DepositRepository,
	withdraws WithdrawRepository,
	addresses DepositAddressResolver,
	publisher EventPublisher,
	txManager TxManager,
	clock Clock,
	idgen IDGenerator,
	chainRules []ChainRule,
) (*Service, error) {
	if wallet == nil || deposits == nil || withdraws == nil || addresses == nil || publisher == nil || txManager == nil || clock == nil || idgen == nil {
		return nil, fmt.Errorf("%w: indexer dependencies are required", errorsx.ErrInvalidArgument)
	}
	chains := make(map[int64]ChainRule, len(chainRules))
	for _, rule := range chainRules {
		if rule.ChainID <= 0 || rule.Asset == "" || rule.RequiredConfirmations <= 0 {
			return nil, fmt.Errorf("%w: invalid chain rule", errorsx.ErrInvalidArgument)
		}
		vault, err := authx.NormalizeEVMAddress(rule.VaultAddress)
		if err != nil {
			return nil, err
		}
		token, err := authx.NormalizeEVMAddress(rule.TokenAddress)
		if err != nil {
			return nil, err
		}
		rule.VaultAddress = vault
		rule.TokenAddress = token
		if strings.TrimSpace(rule.FactoryAddress) != "" {
			factory, err := authx.NormalizeEVMAddress(rule.FactoryAddress)
			if err != nil {
				return nil, err
			}
			rule.FactoryAddress = factory
		}
		chains[rule.ChainID] = rule
	}
	return &Service{
		wallet:    wallet,
		deposits:  deposits,
		withdraws: withdraws,
		addresses: addresses,
		publisher: publisher,
		txManager: txManager,
		clock:     clock,
		idgen:     idgen,
		chains:    chains,
	}, nil
}

func (s *Service) HandleRouterCreated(ctx context.Context, event RouterCreated) error {
	if event.ChainID <= 0 || event.UserID == 0 || event.RouterAddress == "" {
		return fmt.Errorf("%w: invalid router created event", errorsx.ErrInvalidArgument)
	}
	rule, ok := s.chains[event.ChainID]
	if !ok {
		return fmt.Errorf("%w: unsupported chain %d", errorsx.ErrInvalidArgument, event.ChainID)
	}
	routerAddress, err := authx.NormalizeEVMAddress(event.RouterAddress)
	if err != nil {
		return err
	}
	if strings.TrimSpace(rule.FactoryAddress) != "" {
		factoryAddress, err := authx.NormalizeEVMAddress(event.FactoryAddress)
		if err != nil {
			return err
		}
		if factoryAddress != rule.FactoryAddress {
			return s.publishAnomaly(ctx, "unknown_factory", event.TraceID, map[string]any{
				"chain_id":         event.ChainID,
				"factory_address":  factoryAddress,
				"expected_factory": rule.FactoryAddress,
				"user_id":          event.UserID,
				"tx_hash":          event.TxHash,
			})
		}
	}
	return s.addresses.AssignToUser(ctx, event.UserID, event.ChainID, rule.Asset, routerAddress)
}

func (s *Service) HandleDepositObserved(ctx context.Context, event DepositObserved) error {
	if event.Removed {
		return s.handleDepositRemoved(ctx, event)
	}
	rule, routerAddress, err := s.validateDepositEvent(event)
	if err != nil {
		var anomaly anomalyError
		if errors.As(err, &anomaly) {
			return s.publishAnomaly(ctx, anomaly.kind, event.TraceID, anomaly.payload)
		}
		return err
	}

	depositAddress, err := s.addresses.GetByChainAddress(ctx, event.ChainID, routerAddress)
	if err != nil {
		if errors.Is(err, errorsx.ErrNotFound) {
			return s.publishAnomaly(ctx, "unknown_router", event.TraceID, map[string]any{
				"chain_id":       event.ChainID,
				"router_address": routerAddress,
				"tx_hash":        event.TxHash,
				"log_index":      event.LogIndex,
			})
		}
		return err
	}
	if !strings.EqualFold(depositAddress.Asset, rule.Asset) || depositAddress.Status != "ACTIVE" {
		return s.publishAnomaly(ctx, "inactive_or_mismatched_router", event.TraceID, map[string]any{
			"chain_id":       event.ChainID,
			"router_address": routerAddress,
			"router_asset":   depositAddress.Asset,
			"expected_asset": rule.Asset,
			"status":         depositAddress.Status,
			"tx_hash":        event.TxHash,
			"log_index":      event.LogIndex,
		})
	}
	if event.UserID != 0 && event.UserID != depositAddress.UserID {
		return s.publishAnomaly(ctx, "router_user_mismatch", event.TraceID, map[string]any{
			"chain_id":         event.ChainID,
			"router_address":   routerAddress,
			"event_user_id":    event.UserID,
			"address_user_id":  depositAddress.UserID,
			"tx_hash":          event.TxHash,
			"log_index":        event.LogIndex,
		})
	}

	effectiveUserID := depositAddress.UserID
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		deposit, previousStatus, isNew, err := s.ensureDeposit(txCtx, event, effectiveUserID, rule)
		if err != nil {
			return err
		}

		if isNew {
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.deposit.detected",
				AggregateType: "deposit",
				AggregateID:   deposit.DepositID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload:       s.depositPayload(deposit, event, deposit.Status, ""),
			}); err != nil {
				return err
			}
		}

		targetConfirmations := maxInt(deposit.Confirmations, event.Confirmations)
		targetStatus := depositStatusFor(targetConfirmations, rule.RequiredConfirmations)
		if deposit.Status != walletdomain.StatusCredited && deposit.Status != walletdomain.StatusReorgReversed && deposit.Status != targetStatus {
			if err := s.wallet.AdvanceDeposit(txCtx, walletdomain.AdvanceDepositInput{
				DepositID:      deposit.DepositID,
				Confirmations:  targetConfirmations,
				RequiredConfs:  rule.RequiredConfirmations,
				IdempotencyKey: fmt.Sprintf("deposit_advance:%d:%s:%d:%s", event.ChainID, event.TxHash, event.LogIndex, targetStatus),
				TraceID:        event.TraceID,
			}); err != nil {
				return err
			}
			deposit.Status = targetStatus
			deposit.Confirmations = targetConfirmations
		}

		if deposit.Status == walletdomain.StatusConfirming && previousStatus != walletdomain.StatusConfirming {
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.deposit.confirming",
				AggregateType: "deposit",
				AggregateID:   deposit.DepositID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload:       s.depositPayload(deposit, event, walletdomain.StatusConfirming, ""),
			}); err != nil {
				return err
			}
		}

		if deposit.Status == walletdomain.StatusCreditReady && previousStatus != walletdomain.StatusCreditReady {
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.deposit.credit_ready",
				AggregateType: "deposit",
				AggregateID:   deposit.DepositID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload:       s.depositPayload(deposit, event, walletdomain.StatusCreditReady, ""),
			}); err != nil {
				return err
			}
		}

		if deposit.Status == walletdomain.StatusCreditReady {
			if err := s.wallet.ConfirmDeposit(txCtx, walletdomain.ConfirmDepositInput{
				DepositID:      deposit.DepositID,
				IdempotencyKey: fmt.Sprintf("deposit_credit:%d:%s:%d", event.ChainID, event.TxHash, event.LogIndex),
				TraceID:        event.TraceID,
			}); err != nil {
				return err
			}
			credited, err := s.deposits.GetByID(txCtx, deposit.DepositID)
			if err != nil {
				return err
			}
			deposit = credited
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.deposit.credited",
				AggregateType: "deposit",
				AggregateID:   deposit.DepositID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload:       s.depositPayload(deposit, event, walletdomain.StatusCredited, deposit.CreditedLedgerTxID),
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) ReconcileDeposits(ctx context.Context, chainID int64, latestBlock int64, limit int) error {
	rule, ok := s.chains[chainID]
	if !ok {
		return fmt.Errorf("%w: unsupported chain %d", errorsx.ErrInvalidArgument, chainID)
	}
	pending, err := s.deposits.ListPendingByChain(ctx, chainID, []string{
		walletdomain.StatusDetected,
		walletdomain.StatusConfirming,
		walletdomain.StatusCreditReady,
	}, limit)
	if err != nil {
		return err
	}

	for _, deposit := range pending {
		confirmations := calculateConfirmations(latestBlock, deposit.BlockNumber)
		if confirmations <= 0 {
			continue
		}
		event := DepositObserved{
			ChainID:       deposit.ChainID,
			UserID:        deposit.UserID,
			TxHash:        deposit.TxHash,
			LogIndex:      deposit.LogIndex,
			BlockNumber:   deposit.BlockNumber,
			Confirmations: confirmations,
			RouterAddress: deposit.ToAddress,
			VaultAddress:  rule.VaultAddress,
			TokenAddress:  rule.TokenAddress,
			FromAddress:   deposit.FromAddress,
			Amount:        deposit.Amount,
			TraceID:       fmt.Sprintf("reconcile:%s", deposit.DepositID),
			ObservedAt:    s.clock.Now(),
		}
		if err := s.HandleDepositObserved(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HandleWithdrawExecuted(ctx context.Context, event WithdrawExecuted) error {
	rule, err := s.validateWithdrawExecution(event)
	if err != nil {
		var anomaly anomalyError
		if errors.As(err, &anomaly) {
			return s.publishAnomaly(ctx, anomaly.kind, event.TraceID, anomaly.payload)
		}
		return err
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		withdraw, err := s.withdraws.GetByID(txCtx, event.WithdrawID)
		if err != nil {
			if errors.Is(err, errorsx.ErrNotFound) {
				return s.publishAnomaly(txCtx, "unknown_withdraw_execution", event.TraceID, map[string]any{
					"chain_id":    event.ChainID,
					"withdraw_id": event.WithdrawID,
					"tx_hash":     event.TxHash,
				})
			}
			return err
		}

		switch withdraw.Status {
		case walletdomain.StatusCompleted, walletdomain.StatusRefunded:
			return nil
		case walletdomain.StatusApproved:
			if err := s.wallet.MarkWithdrawBroadcasted(txCtx, walletdomain.BroadcastWithdrawInput{
				WithdrawID:     withdraw.WithdrawID,
				TxHash:         event.TxHash,
				IdempotencyKey: fmt.Sprintf("withdraw_broadcast:%s:%s", withdraw.WithdrawID, event.TxHash),
				TraceID:        event.TraceID,
			}); err != nil {
				return err
			}
			withdraw.Status = walletdomain.StatusBroadcasted
			withdraw.BroadcastTxHash = event.TxHash
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.withdraw.broadcasted",
				AggregateType: "withdraw",
				AggregateID:   withdraw.WithdrawID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload:       s.withdrawPayload(withdraw, event.TxHash, walletdomain.StatusBroadcasted),
			}); err != nil {
				return err
			}
		case walletdomain.StatusBroadcasted:
			if withdraw.BroadcastTxHash == "" {
				withdraw.BroadcastTxHash = event.TxHash
			}
		default:
			return s.publishAnomaly(txCtx, "withdraw_execution_invalid_state", event.TraceID, map[string]any{
				"chain_id":    event.ChainID,
				"withdraw_id": withdraw.WithdrawID,
				"tx_hash":     event.TxHash,
				"status":      withdraw.Status,
			})
		}

		if !strings.EqualFold(withdraw.Asset, rule.Asset) {
			return s.publishAnomaly(txCtx, "withdraw_asset_mismatch", event.TraceID, map[string]any{
				"chain_id":       event.ChainID,
				"withdraw_id":    withdraw.WithdrawID,
				"withdraw_asset": withdraw.Asset,
				"expected_asset": rule.Asset,
				"tx_hash":        event.TxHash,
			})
		}
		if err := s.wallet.CompleteWithdraw(txCtx, walletdomain.CompleteWithdrawInput{
			WithdrawID:     withdraw.WithdrawID,
			TxHash:         event.TxHash,
			IdempotencyKey: fmt.Sprintf("withdraw_complete:%s:%s", withdraw.WithdrawID, event.TxHash),
			TraceID:        event.TraceID,
		}); err != nil {
			return err
		}
		return s.publish(txCtx, EventEnvelope{
			EventID:       s.idgen.NewID("evt"),
			EventType:     "wallet.withdraw.completed",
			AggregateType: "withdraw",
			AggregateID:   withdraw.WithdrawID,
			TraceID:       event.TraceID,
			Producer:      "indexer",
			Version:       1,
			OccurredAt:    s.occurredAt(event.ObservedAt),
			Payload:       s.withdrawPayload(withdraw, event.TxHash, walletdomain.StatusCompleted),
		})
	})
}

func (s *Service) HandleWithdrawFailed(ctx context.Context, event WithdrawFailed) error {
	if event.ChainID <= 0 || event.WithdrawID == "" || event.TxHash == "" {
		return fmt.Errorf("%w: invalid withdraw failure event", errorsx.ErrInvalidArgument)
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		withdraw, err := s.withdraws.GetByID(txCtx, event.WithdrawID)
		if err != nil {
			if errors.Is(err, errorsx.ErrNotFound) {
				return s.publishAnomaly(txCtx, "unknown_withdraw_failure", event.TraceID, map[string]any{
					"chain_id":    event.ChainID,
					"withdraw_id": event.WithdrawID,
					"tx_hash":     event.TxHash,
					"reason":      event.Reason,
				})
			}
			return err
		}

		switch withdraw.Status {
		case walletdomain.StatusCompleted:
			return s.publishAnomaly(txCtx, "withdraw_failed_after_completion", event.TraceID, map[string]any{
				"chain_id":    event.ChainID,
				"withdraw_id": withdraw.WithdrawID,
				"tx_hash":     event.TxHash,
			})
		case walletdomain.StatusRefunded:
			return nil
		case walletdomain.StatusApproved, walletdomain.StatusBroadcasted:
			if err := s.withdraws.UpdateStatus(txCtx, withdraw.WithdrawID, []string{withdraw.Status}, walletdomain.StatusFailed); err != nil {
				return err
			}
			if err := s.publish(txCtx, EventEnvelope{
				EventID:       s.idgen.NewID("evt"),
				EventType:     "wallet.withdraw.failed",
				AggregateType: "withdraw",
				AggregateID:   withdraw.WithdrawID,
				TraceID:       event.TraceID,
				Producer:      "indexer",
				Version:       1,
				OccurredAt:    s.occurredAt(event.ObservedAt),
				Payload: map[string]any{
					"withdraw_id": withdraw.WithdrawID,
					"chain_id":    withdraw.ChainID,
					"tx_hash":     event.TxHash,
					"reason":      event.Reason,
					"status":      walletdomain.StatusFailed,
				},
			}); err != nil {
				return err
			}
		case walletdomain.StatusFailed:
		default:
			return s.publishAnomaly(txCtx, "withdraw_failure_invalid_state", event.TraceID, map[string]any{
				"chain_id":    event.ChainID,
				"withdraw_id": withdraw.WithdrawID,
				"tx_hash":     event.TxHash,
				"status":      withdraw.Status,
				"reason":      event.Reason,
			})
		}

		return s.wallet.RefundWithdraw(txCtx, walletdomain.RefundWithdrawInput{
			WithdrawID:     withdraw.WithdrawID,
			IdempotencyKey: fmt.Sprintf("withdraw_refund:%s:%s", withdraw.WithdrawID, event.TxHash),
			TraceID:        event.TraceID,
		})
	})
}

func (s *Service) handleDepositRemoved(ctx context.Context, event DepositObserved) error {
	if event.ChainID <= 0 || event.TxHash == "" {
		return fmt.Errorf("%w: invalid removed deposit event", errorsx.ErrInvalidArgument)
	}
	return s.txManager.WithinTransaction(ctx, func(txCtx context.Context) error {
		deposit, err := s.deposits.GetByTxLog(txCtx, event.ChainID, event.TxHash, event.LogIndex)
		if err != nil {
			if errors.Is(err, errorsx.ErrNotFound) {
				return nil
			}
			return err
		}
		if deposit.Status == walletdomain.StatusCredited {
			return s.publishAnomaly(txCtx, "credited_deposit_reorg", event.TraceID, map[string]any{
				"chain_id":   event.ChainID,
				"deposit_id": deposit.DepositID,
				"tx_hash":    event.TxHash,
				"log_index":  event.LogIndex,
			})
		}
		if err := s.wallet.ReverseDeposit(txCtx, walletdomain.ReverseDepositInput{
			DepositID:      deposit.DepositID,
			IdempotencyKey: fmt.Sprintf("deposit_reorg:%d:%s:%d", event.ChainID, event.TxHash, event.LogIndex),
			TraceID:        event.TraceID,
		}); err != nil {
			return err
		}
		return s.publishAnomaly(txCtx, "deposit_reorg_reversed", event.TraceID, map[string]any{
			"chain_id":   event.ChainID,
			"deposit_id": deposit.DepositID,
			"tx_hash":    event.TxHash,
			"log_index":  event.LogIndex,
			"status":     walletdomain.StatusReorgReversed,
		})
	})
}

func (s *Service) ensureDeposit(ctx context.Context, event DepositObserved, userID uint64, rule ChainRule) (walletdomain.DepositChainTx, string, bool, error) {
	existing, err := s.deposits.GetByTxLog(ctx, event.ChainID, event.TxHash, event.LogIndex)
	if err == nil {
		return existing, existing.Status, false, nil
	}
	if !errors.Is(err, errorsx.ErrNotFound) {
		return walletdomain.DepositChainTx{}, "", false, err
	}

	deposit, err := s.wallet.DetectDeposit(ctx, walletdomain.DetectDepositInput{
		UserID:         userID,
		ChainID:        event.ChainID,
		TxHash:         event.TxHash,
		LogIndex:       event.LogIndex,
		FromAddress:    event.FromAddress,
		ToAddress:      event.RouterAddress,
		TokenAddress:   rule.TokenAddress,
		Amount:         event.Amount,
		Asset:          rule.Asset,
		BlockNumber:    event.BlockNumber,
		Confirmations:  event.Confirmations,
		RequiredConfs:  rule.RequiredConfirmations,
		IdempotencyKey: fmt.Sprintf("deposit_detected:%d:%s:%d", event.ChainID, event.TxHash, event.LogIndex),
		TraceID:        event.TraceID,
	})
	if err != nil {
		return walletdomain.DepositChainTx{}, "", false, err
	}
	return deposit, "", true, nil
}

func (s *Service) depositPayload(deposit walletdomain.DepositChainTx, event DepositObserved, status string, ledgerTxID string) map[string]any {
	payload := map[string]any{
		"deposit_id":     deposit.DepositID,
		"user_id":        deposit.UserID,
		"chain_id":       deposit.ChainID,
		"tx_hash":        deposit.TxHash,
		"log_index":      deposit.LogIndex,
		"block_number":   deposit.BlockNumber,
		"router_address": event.RouterAddress,
		"vault_address":  event.VaultAddress,
		"token_address":  event.TokenAddress,
		"asset":          deposit.Asset,
		"amount":         deposit.Amount,
		"confirmations":  maxInt(deposit.Confirmations, event.Confirmations),
		"status":         status,
	}
	if ledgerTxID != "" {
		payload["ledger_tx_id"] = ledgerTxID
	}
	return payload
}

func (s *Service) withdrawPayload(withdraw walletdomain.WithdrawRequest, txHash string, status string) map[string]any {
	netAmount, _ := netWithdrawAmount(withdraw.Amount, withdraw.FeeAmount)
	return map[string]any{
		"withdraw_id":  withdraw.WithdrawID,
		"user_id":      withdraw.UserID,
		"chain_id":     withdraw.ChainID,
		"tx_hash":      txHash,
		"to_address":   withdraw.ToAddress,
		"asset":        withdraw.Asset,
		"gross_amount": withdraw.Amount,
		"net_amount":   netAmount,
		"fee_amount":   withdraw.FeeAmount,
		"status":       status,
	}
}

func (s *Service) validateDepositEvent(event DepositObserved) (ChainRule, string, error) {
	if event.ChainID <= 0 || event.TxHash == "" || event.Amount == "" || event.RouterAddress == "" {
		return ChainRule{}, "", fmt.Errorf("%w: invalid deposit event", errorsx.ErrInvalidArgument)
	}
	rule, ok := s.chains[event.ChainID]
	if !ok {
		return ChainRule{}, "", fmt.Errorf("%w: unsupported chain %d", errorsx.ErrInvalidArgument, event.ChainID)
	}
	router, err := authx.NormalizeEVMAddress(event.RouterAddress)
	if err != nil {
		return ChainRule{}, "", err
	}
	vault, err := authx.NormalizeEVMAddress(event.VaultAddress)
	if err != nil {
		return ChainRule{}, "", err
	}
	token, err := authx.NormalizeEVMAddress(event.TokenAddress)
	if err != nil {
		return ChainRule{}, "", err
	}
	if vault != rule.VaultAddress {
		return ChainRule{}, "", anomalyError{kind: "unknown_vault", payload: map[string]any{
			"chain_id":         event.ChainID,
			"vault_address":    vault,
			"expected_address": rule.VaultAddress,
			"tx_hash":          event.TxHash,
			"log_index":        event.LogIndex,
		}}
	}
	if token != rule.TokenAddress {
		return ChainRule{}, "", anomalyError{kind: "unknown_token", payload: map[string]any{
			"chain_id":         event.ChainID,
			"token_address":    token,
			"expected_address": rule.TokenAddress,
			"tx_hash":          event.TxHash,
			"log_index":        event.LogIndex,
		}}
	}
	return rule, router, nil
}

func (s *Service) validateWithdrawExecution(event WithdrawExecuted) (ChainRule, error) {
	if event.ChainID <= 0 || event.WithdrawID == "" || event.TxHash == "" {
		return ChainRule{}, fmt.Errorf("%w: invalid withdraw execution event", errorsx.ErrInvalidArgument)
	}
	rule, ok := s.chains[event.ChainID]
	if !ok {
		return ChainRule{}, fmt.Errorf("%w: unsupported chain %d", errorsx.ErrInvalidArgument, event.ChainID)
	}
	vault, err := authx.NormalizeEVMAddress(event.VaultAddress)
	if err != nil {
		return ChainRule{}, err
	}
	token, err := authx.NormalizeEVMAddress(event.TokenAddress)
	if err != nil {
		return ChainRule{}, err
	}
	if vault != rule.VaultAddress {
		return ChainRule{}, anomalyError{kind: "unknown_withdraw_vault", payload: map[string]any{
			"chain_id":         event.ChainID,
			"vault_address":    vault,
			"expected_address": rule.VaultAddress,
			"withdraw_id":      event.WithdrawID,
			"tx_hash":          event.TxHash,
		}}
	}
	if token != rule.TokenAddress {
		return ChainRule{}, anomalyError{kind: "unknown_withdraw_token", payload: map[string]any{
			"chain_id":         event.ChainID,
			"token_address":    token,
			"expected_address": rule.TokenAddress,
			"withdraw_id":      event.WithdrawID,
			"tx_hash":          event.TxHash,
		}}
	}
	return rule, nil
}

func (s *Service) publish(ctx context.Context, envelope EventEnvelope) error {
	if envelope.EventID == "" {
		envelope.EventID = s.idgen.NewID("evt")
	}
	if envelope.Producer == "" {
		envelope.Producer = "indexer"
	}
	if envelope.Version == 0 {
		envelope.Version = 1
	}
	if envelope.OccurredAt.IsZero() {
		envelope.OccurredAt = s.clock.Now()
	}
	return s.publisher.Publish(ctx, envelope)
}

func (s *Service) publishAnomaly(ctx context.Context, kind string, traceID string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["kind"] = kind
	return s.publish(ctx, EventEnvelope{
		EventType:     "wallet.indexer.anomaly",
		AggregateType: "chain_event",
		AggregateID:   s.idgen.NewID("anom"),
		TraceID:       traceID,
		Payload:       payload,
	})
}

func (s *Service) occurredAt(at time.Time) time.Time {
	if at.IsZero() {
		return s.clock.Now()
	}
	return at
}

func depositStatusFor(confirmations int, required int) string {
	if confirmations >= required && required > 0 {
		return walletdomain.StatusCreditReady
	}
	if confirmations > 0 {
		return walletdomain.StatusConfirming
	}
	return walletdomain.StatusDetected
}

func calculateConfirmations(latestBlock int64, blockNumber int64) int {
	if latestBlock < blockNumber || blockNumber <= 0 {
		return 0
	}
	return int(latestBlock - blockNumber + 1)
}

func netWithdrawAmount(amount string, fee string) (string, error) {
	base, err := decimalx.NewFromString(amount)
	if err != nil {
		return "", err
	}
	cost, err := decimalx.NewFromString(fee)
	if err != nil {
		return "", err
	}
	if base.LessThan(cost) {
		return "", fmt.Errorf("%w: fee exceeds amount", errorsx.ErrInvalidArgument)
	}
	return base.Sub(cost).String(), nil
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

type Runner struct {
	source     EventSource
	service    *Service
	cursors    CursorRepository
	clock      Clock
	chainRules map[int64]ChainRule
	batchSize  int64
}

func NewRunner(source EventSource, service *Service, cursors CursorRepository, clock Clock, chainRules []ChainRule, batchSize int64) (*Runner, error) {
	if source == nil || service == nil || cursors == nil || clock == nil {
		return nil, fmt.Errorf("%w: runner dependencies are required", errorsx.ErrInvalidArgument)
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	rules := make(map[int64]ChainRule, len(chainRules))
	for _, rule := range chainRules {
		rules[rule.ChainID] = rule
	}
	return &Runner{
		source:     source,
		service:    service,
		cursors:    cursors,
		clock:      clock,
		chainRules: rules,
		batchSize:  batchSize,
	}, nil
}

func (r *Runner) SyncChain(ctx context.Context, chainID int64) error {
	if _, ok := r.chainRules[chainID]; !ok {
		return fmt.Errorf("%w: unsupported chain %d", errorsx.ErrInvalidArgument, chainID)
	}
	latestBlock, err := r.source.LatestBlockNumber(ctx, chainID)
	if err != nil {
		return err
	}
	if err := r.syncRouters(ctx, chainID, latestBlock); err != nil {
		return err
	}
	if err := r.syncDeposits(ctx, chainID, latestBlock); err != nil {
		return err
	}
	if err := r.syncWithdraws(ctx, chainID, latestBlock); err != nil {
		return err
	}
	if err := r.syncWithdrawFailures(ctx, chainID); err != nil {
		return err
	}
	return r.service.ReconcileDeposits(ctx, chainID, latestBlock, int(r.batchSize))
}

func (r *Runner) syncRouters(ctx context.Context, chainID int64, latestBlock int64) error {
	fromBlock, toBlock, err := r.scanRange(ctx, chainID, CursorTypeRouterScan, latestBlock)
	if err != nil || fromBlock > toBlock {
		return err
	}
	events, err := r.source.ListRouterCreatedEvents(ctx, chainID, fromBlock, toBlock)
	if err != nil {
		return err
	}
	sort.Slice(events, func(i int, j int) bool {
		if events[i].BlockNumber == events[j].BlockNumber {
			return events[i].LogIndex < events[j].LogIndex
		}
		return events[i].BlockNumber < events[j].BlockNumber
	})
	for _, event := range events {
		if err := r.service.HandleRouterCreated(ctx, event); err != nil {
			return err
		}
	}
	return r.cursors.Upsert(ctx, chainID, CursorTypeRouterScan, fmt.Sprintf("%d", toBlock), r.clock.Now())
}

func (r *Runner) syncDeposits(ctx context.Context, chainID int64, latestBlock int64) error {
	fromBlock, toBlock, err := r.scanRange(ctx, chainID, CursorTypeDepositScan, latestBlock)
	if err != nil || fromBlock > toBlock {
		return err
	}
	events, err := r.source.ListDepositEvents(ctx, chainID, fromBlock, toBlock)
	if err != nil {
		return err
	}
	sort.Slice(events, func(i int, j int) bool {
		if events[i].BlockNumber == events[j].BlockNumber {
			return events[i].LogIndex < events[j].LogIndex
		}
		return events[i].BlockNumber < events[j].BlockNumber
	})
	for i := range events {
		events[i].Confirmations = calculateConfirmations(latestBlock, events[i].BlockNumber)
		if err := r.service.HandleDepositObserved(ctx, events[i]); err != nil {
			return err
		}
	}
	return r.cursors.Upsert(ctx, chainID, CursorTypeDepositScan, fmt.Sprintf("%d", toBlock), r.clock.Now())
}

func (r *Runner) syncWithdraws(ctx context.Context, chainID int64, latestBlock int64) error {
	fromBlock, toBlock, err := r.scanRange(ctx, chainID, CursorTypeWithdrawScan, latestBlock)
	if err != nil || fromBlock > toBlock {
		return err
	}
	events, err := r.source.ListWithdrawEvents(ctx, chainID, fromBlock, toBlock)
	if err != nil {
		return err
	}
	sort.Slice(events, func(i int, j int) bool {
		if events[i].BlockNumber == events[j].BlockNumber {
			return events[i].LogIndex < events[j].LogIndex
		}
		return events[i].BlockNumber < events[j].BlockNumber
	})
	for _, event := range events {
		if err := r.service.HandleWithdrawExecuted(ctx, event); err != nil {
			return err
		}
	}
	return r.cursors.Upsert(ctx, chainID, CursorTypeWithdrawScan, fmt.Sprintf("%d", toBlock), r.clock.Now())
}

func (r *Runner) syncWithdrawFailures(ctx context.Context, chainID int64) error {
	pending, err := r.service.withdraws.ListByChainStatuses(ctx, chainID, []string{walletdomain.StatusBroadcasted}, int(r.batchSize))
	if err != nil {
		return err
	}
	for _, withdraw := range pending {
		if strings.TrimSpace(withdraw.BroadcastTxHash) == "" {
			continue
		}
		receipt, err := r.source.GetReceiptStatus(ctx, chainID, withdraw.BroadcastTxHash)
		if err != nil {
			return err
		}
		if !receipt.Found || receipt.Success {
			continue
		}
		if err := r.service.HandleWithdrawFailed(ctx, WithdrawFailed{
			ChainID:    chainID,
			WithdrawID: withdraw.WithdrawID,
			TxHash:     withdraw.BroadcastTxHash,
			Reason:     "receipt_status_failed",
			ObservedAt: r.clock.Now(),
			TraceID:    fmt.Sprintf("receipt:%s", withdraw.WithdrawID),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runner) scanRange(ctx context.Context, chainID int64, cursorType string, latestBlock int64) (int64, int64, error) {
	cursor, err := r.cursors.Get(ctx, chainID, cursorType)
	start := int64(0)
	if err != nil && !errors.Is(err, errorsx.ErrNotFound) {
		return 0, 0, err
	}
	if err == nil && cursor.CursorValue != "" {
		if _, scanErr := fmt.Sscanf(cursor.CursorValue, "%d", &start); scanErr != nil {
			return 0, 0, scanErr
		}
	}
	fromBlock := start + 1
	if fromBlock > latestBlock {
		return 0, -1, nil
	}
	toBlock := latestBlock
	if maxBlock := fromBlock + r.batchSize - 1; maxBlock < toBlock {
		toBlock = maxBlock
	}
	return fromBlock, toBlock, nil
}

type anomalyError struct {
	kind    string
	payload map[string]any
}

func (e anomalyError) Error() string {
	return e.kind
}
