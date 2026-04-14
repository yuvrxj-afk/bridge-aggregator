package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/store"

	"github.com/google/uuid"
)

var (
	ErrRouteRequired       = errors.New("route is required in request body")
	ErrRouteHopsEmpty      = errors.New("route must have at least one hop")
	ErrUnknownBridgeID     = errors.New("route hop bridge_id is not a registered adapter")
	ErrNoBridgeHop         = errors.New("route must contain at least one bridge hop")
	ErrQuoteExpiryRequired = errors.New("route.quote_expires_at is required")
	ErrQuoteExpired        = errors.New("route quote has expired")
	ErrInvalidStatus       = errors.New("status must be one of: submitted, completed, failed")
	ErrInvalidTransition   = errors.New("invalid operation status transition")
	ErrTxHashRequired      = errors.New("tx_hash is required when status=submitted")
)

var validTransitionStatuses = map[string]bool{
	models.OperationStatusSubmitted: true,
	models.OperationStatusCompleted: true,
	models.OperationStatusFailed:    true,
}

// ValidateExecuteRequest checks route presence, non-empty hops, and that at least one bridge hop's bridge_id is registered.
func ValidateExecuteRequest(req models.ExecuteRequest, adapterIDs map[string]bool) error {
	if req.Route == nil {
		return ErrRouteRequired
	}
	if len(req.Route.Hops) == 0 {
		return ErrRouteHopsEmpty
	}
	if strings.TrimSpace(req.Route.QuoteExpiresAt) == "" {
		return ErrQuoteExpiryRequired
	}
	exp, err := time.Parse(time.RFC3339, req.Route.QuoteExpiresAt)
	if err != nil {
		return ErrQuoteExpiryRequired
	}
	if time.Now().UTC().After(exp) {
		return ErrQuoteExpired
	}

	// Execute is still bridge-centric (recording a bridge operation). For composed routes,
	// we require at least one bridge hop whose BridgeID matches a registered bridge adapter.
	for _, h := range req.Route.Hops {
		hopType := h.HopType
		if hopType == "" {
			hopType = models.HopTypeBridge
		}
		if hopType != models.HopTypeBridge {
			continue
		}
		if h.BridgeID != "" && adapterIDs[h.BridgeID] {
			return nil
		}
		return ErrUnknownBridgeID
	}
	return ErrNoBridgeHop
}

// Execute creates an operation for the given route (idempotent when idempotency_key is provided).
// It does not submit on-chain transactions; it only persists the operation with status pending.
func Execute(ctx context.Context, s *store.Store, adapters []bridges.Adapter, req models.ExecuteRequest) (*models.ExecuteResponse, error) {
	adapterIDs := make(map[string]bool)
	for _, a := range adapters {
		adapterIDs[a.ID()] = true
	}
	if err := ValidateExecuteRequest(req, adapterIDs); err != nil {
		return nil, err
	}

	if req.IdempotencyKey != "" {
		existing, err := s.GetOperationByIdempotencyKey(req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return &models.ExecuteResponse{
				OperationID:       existing.ID,
				Status:            existing.Status,
				ClientReferenceID: existing.ClientReferenceID,
			}, nil
		}
	}

	opID := uuid.NewString()
	op := store.Operation{
		ID:                opID,
		Route:             *req.Route,
		Status:            models.OperationStatusPending,
		WalletAddress:     req.WalletAddress,
		ClientReferenceID: req.ClientReferenceID,
		IdempotencyKey:    req.IdempotencyKey,
	}

	created, err := s.CreateOperation(op)
	if err != nil {
		return nil, err
	}

	return &models.ExecuteResponse{
		OperationID:       created.ID,
		Status:            created.Status,
		ClientReferenceID: created.ClientReferenceID,
	}, nil
}

// GetOperation loads an operation by ID and returns the API response shape.
func GetOperation(ctx context.Context, s *store.Store, id string) (*models.OperationResponse, error) {
	op, err := s.GetOperation(id)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, nil
	}
	return &models.OperationResponse{
		OperationID:       op.ID,
		Status:            op.Status,
		TxHash:            op.TxHash,
		Route:             op.Route,
		ClientReferenceID: op.ClientReferenceID,
		CreatedAt:         op.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         op.UpdatedAt.Format(time.RFC3339),
		NextAction:        nextAction(op.Status),
		RecoveryHints:     recoveryHints(op.Status, op.TxHash),
	}, nil
}

// UpdateOperationStatus transitions an operation to a new status.
// Only submitted/completed/failed are allowed; pending is the initial state set by Execute.
func UpdateOperationStatus(ctx context.Context, s *store.Store, id string, req models.UpdateOperationStatusRequest) error {
	if !validTransitionStatuses[req.Status] {
		return ErrInvalidStatus
	}
	op, err := s.GetOperation(id)
	if err != nil {
		return err
	}
	if op == nil {
		return store.ErrNotFound
	}
	if req.Status == models.OperationStatusSubmitted && req.TxHash == "" {
		return ErrTxHashRequired
	}
	if !isValidTransition(op.Status, req.Status) {
		return ErrInvalidTransition
	}
	if err := s.UpdateOperationStatus(id, req.Status, req.TxHash); err != nil {
		return err
	}
	return nil
}

// GetOperationEvents returns persisted lifecycle events for recovery/audit.
func GetOperationEvents(ctx context.Context, s *store.Store, id string, limit int) ([]models.OperationEventResponse, error) {
	_ = ctx
	op, err := s.GetOperation(id)
	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, store.ErrNotFound
	}
	events, err := s.ListOperationEvents(id, limit)
	if err != nil {
		return nil, err
	}
	out := make([]models.OperationEventResponse, 0, len(events))
	for _, e := range events {
		out = append(out, models.OperationEventResponse{
			ID:         e.ID,
			EventType:  e.EventType,
			FromStatus: e.FromStatus,
			ToStatus:   e.ToStatus,
			TxHash:     e.TxHash,
			Metadata:   e.Metadata,
			CreatedAt:  e.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

func isValidTransition(from, to string) bool {
	switch from {
	case models.OperationStatusPending:
		return to == models.OperationStatusSubmitted || to == models.OperationStatusFailed
	case models.OperationStatusSubmitted:
		return to == models.OperationStatusCompleted || to == models.OperationStatusFailed
	case models.OperationStatusCompleted, models.OperationStatusFailed:
		return false
	default:
		return false
	}
}

func nextAction(status string) string {
	switch status {
	case models.OperationStatusPending:
		return "submit_source_transaction"
	case models.OperationStatusSubmitted:
		return "wait_for_confirmation_or_bridge_completion"
	case models.OperationStatusCompleted:
		return "none"
	case models.OperationStatusFailed:
		return "retry_or_requote"
	default:
		return "inspect_operation"
	}
}

func recoveryHints(status, txHash string) []string {
	switch status {
	case models.OperationStatusPending:
		return []string{
			"Use /api/v1/route/buildTransaction or /api/v1/route/stepTransaction to get executable tx data.",
			"After wallet submission, PATCH status to submitted with tx_hash.",
		}
	case models.OperationStatusSubmitted:
		hints := []string{
			"Poll destination bridge explorer or provider UI for fill/finality.",
			"When final settlement succeeds, PATCH status to completed.",
			"If transaction reverted or bridge failed irrecoverably, PATCH status to failed.",
		}
		if txHash == "" {
			hints = append(hints, "Missing tx_hash: include tx_hash on submitted status for reliable recovery.")
		}
		return hints
	case models.OperationStatusFailed:
		return []string{
			"Re-quote before retrying to avoid stale fees/deadlines.",
			"Use idempotency_key on /execute for safe client retries.",
		}
	default:
		return nil
	}
}
