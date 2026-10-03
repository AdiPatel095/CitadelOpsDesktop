package Intent

// ConfirmationRequiredError records a game quote without authorizing confirmation.
type ConfirmationRequiredError struct {
	Response *ResponseCodeError
	QuotedC2 int64
}

func (err *ConfirmationRequiredError) Error() string { return err.Response.Meaning.Message }
func (err *ConfirmationRequiredError) Unwrap() error { return err.Response }
