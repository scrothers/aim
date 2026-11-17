package mail

import "github.com/scrothers/aim/internal/address"

// WithOnOperatorDelivery registers fn to be called after a message addressed
// to the operator (to or cc) has been committed, including messages sent by
// the operator to itself. fn runs on the sender's goroutine and must not
// block; hand work off instead.
func WithOnOperatorDelivery(fn func(Message)) Option {
	return func(s *Service) { s.onOperator = fn }
}

// SetOnOperatorDelivery installs the hook of WithOnOperatorDelivery on an
// existing Service. Call it before the Service handles requests.
func (s *Service) SetOnOperatorDelivery(fn func(Message)) { s.onOperator = fn }

// afterSend runs post-commit hooks for a sent message.
func (s *Service) afterSend(m Message) {
	if s.onOperator == nil {
		return
	}
	for _, a := range m.To {
		if a == address.OperatorName {
			s.onOperator(m)
			return
		}
	}
	for _, a := range m.Cc {
		if a == address.OperatorName {
			s.onOperator(m)
			return
		}
	}
}
