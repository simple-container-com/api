package security

import (
	"fmt"
	"regexp"
)

// imageRefRe validates container image references to prevent argv confusion.
// Accepts: registry/repo:tag, registry/repo@sha256:hex, repo:tag
// Rejects: refs starting with "-" (flag injection) or containing shell metacharacters.
var imageRefRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/:@-]*$`)

// ValidateImageRef checks that an image reference is safe to pass to external tools.
func ValidateImageRef(imageRef string) error {
	if imageRef == "" {
		return fmt.Errorf("image reference is empty")
	}
	if !imageRefRe.MatchString(imageRef) {
		return fmt.Errorf("image reference %q contains invalid characters", imageRef)
	}
	return nil
}

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
