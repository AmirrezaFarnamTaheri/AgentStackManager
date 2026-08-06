package app

import "github.com/agentstack/agentstack/internal/model"

func (s *Service) ListTransactions(limit int) ([]model.Transaction, error) {
	return s.Store.ListTransactions(limit)
}
