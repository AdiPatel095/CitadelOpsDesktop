package Localization

// describedError preserves the original Error/Unwrap chain. Presentation
// metadata never changes error classification or dispatch decisions.
type describedError struct {
	cause   error
	message *Message
}

func (err *describedError) Error() string                 { return err.cause.Error() }
func (err *describedError) Unwrap() error                 { return err.cause }
func (err *describedError) LocalizationMessage() *Message { return Clone(err.message) }

func WithError(err error, message *Message) error {
	if err == nil || message == nil {
		return err
	}
	return &describedError{cause: err, message: Bind(message, err.Error())}
}

func FromError(err error) *Message {
	// Do not walk an unknown outer wrapper: returning a child's descriptor alone
	// would hide the outer validation context or other specific information.
	if source, ok := err.(interface{ LocalizationMessage() *Message }); ok {
		return Clone(source.LocalizationMessage())
	}
	return nil
}

// ErrorContext prefixes a translated validation context to a known cause. Any
// unknown cause or excessive context stays explicitly untranslated in full.
func ErrorContext(context *Message, cause error) *Message {
	return Join(context, FromError(cause))
}
