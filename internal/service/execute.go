package service

import (
	"context"
	"errors"
	"time"

	"bridge-aggregator/internal/bridges"
	"bridge-aggregator/internal/models"
	"bridge-aggregator/internal/store"

	"github.com/google/uuid"
)

var (
	ErrRouteRequired     = errors.New("route is required in request body")
	ErrRouteHopsEmpty    = errors.New("route must have at least one hop")
	ErrUnknownBridgeID   = errors.New("route hop bridge_id is not a registered adapter")
)

// ValidateExecuteRequest checks route presence, non-empty hops, and that the first hop's bridge_id is registered.
func ValidateExecuteRequest(req models.ExecuteRequest, adapterIDs map[string]bool) error {
	if req.Route == nil {
		return ErrRouteRequired
	}
	if len(req.Route.Hops) == 0 {
		return ErrRouteHopsEmpty
	}
	firstBridgeID := req.Route.Hops[0].BridgeID
	if firstBridgeID == "" || !adapterIDs[firstBridgeID] {
		return ErrUnknownBridgeID
	}
	return nil
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
		Route:             op.Route,
		ClientReferenceID: op.ClientReferenceID,
		CreatedAt:         op.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         op.UpdatedAt.Format(time.RFC3339),
	}, nil
}
