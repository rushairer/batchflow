package observability_test

import (
	"errors"
	"fmt"

	"github.com/rushairer/batchflow/v2"
)

type apiRateLimitError struct{}

func (apiRateLimitError) Error() string {
	return "remote api rate limited"
}

func ExampleRegisterErrorClassifier_customBackend() {
	unregister := batchflow.RegisterErrorClassifier(batchflow.ErrorClassifierFunc(
		func(err error) (bool, string, bool) {
			var rateLimit apiRateLimitError
			if errors.As(err, &rateLimit) {
				return true, "rate_limit", true
			}
			return false, "", false
		},
	))
	defer unregister()

	retryable, reason := batchflow.ClassifyError(apiRateLimitError{})
	fmt.Println(retryable, reason)
	// Output:
	// true rate_limit
}
