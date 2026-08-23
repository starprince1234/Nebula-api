package domain

import (
	"errors"
	"fmt"
)

type Error struct {
	Code    string
	Message string
	Details any
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func WrapError(code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func ErrorCode(err error) string {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return "INTERNAL_ERROR"
}

const (
	CodeValidation                 = "VALIDATION_ERROR"
	CodeRejectionReasonRequired    = "REJECTION_REASON_REQUIRED"
	CodeAuthenticationRequired     = "AUTHENTICATION_REQUIRED"
	CodeInvalidCredentials         = "INVALID_CREDENTIALS"
	CodeTokenExpired               = "TOKEN_EXPIRED"
	CodeForbidden                  = "FORBIDDEN"
	CodeAccountDisabled            = "ACCOUNT_DISABLED"
	CodeNotFound                   = "RESOURCE_NOT_FOUND"
	CodeEmailRegistered            = "EMAIL_ALREADY_REGISTERED"
	CodeNameConflict               = "RESOURCE_NAME_CONFLICT"
	CodeInvalidTransition          = "INVALID_STATE_TRANSITION"
	CodeProjectNoMentor            = "PROJECT_HAS_NO_MENTOR"
	CodeMentorNotInOrganization    = "MENTOR_NOT_IN_ORGANIZATION"
	CodeMentorAlreadyProjectMember = "MENTOR_ALREADY_PROJECT_MEMBER"
	CodeModelNotReady              = "MODEL_NOT_READY"
	CodeModelRoutingRequired       = "MODEL_ROUTING_REQUIRED"
	CodeKeyAlreadyClaimed          = "KEY_ALREADY_CLAIMED"
	CodeVerificationInvalid        = "VERIFICATION_CODE_INVALID"
	CodeVerificationExpired        = "VERIFICATION_CODE_EXPIRED"
	CodeInvitationInvalid          = "INVITATION_INVALID"
	CodeRateLimited                = "RATE_LIMITED"
	CodeVerificationLocked         = "VERIFICATION_LOCKED"
	CodeDependencyUnavailable      = "DEPENDENCY_UNAVAILABLE"
)
