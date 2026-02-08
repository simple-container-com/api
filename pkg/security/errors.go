package security

import "fmt"

// SecurityError represents a security operation error
type SecurityError struct {
	Operation string
	Err       error
	Message   string
}

func (e *SecurityError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s failed: %s: %v", e.Operation, e.Message, e.Err)
	}
	return fmt.Sprintf("%s failed: %s", e.Operation, e.Message)
}

func (e *SecurityError) Unwrap() error {
	return e.Err
}

// NewSecurityError creates a new security error
func NewSecurityError(operation, message string, err error) *SecurityError {
	return &SecurityError{
		Operation: operation,
		Message:   message,
		Err:       err,
	}
}
