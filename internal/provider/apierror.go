package provider

import (
	"errors"

	acsdk "github.com/American-Cloud/americancloud-sdk-go"
	"github.com/American-Cloud/americancloud-sdk-go/core"
)

// isNotFound reports whether err is a 404 from the SDK. GET-backed resources use
// it in Read to detect a resource deleted out-of-band and drop it from state
// (so Terraform recreates it) rather than erroring.
func isNotFound(err error) bool {
	var nf *acsdk.NotFoundError
	return errors.As(err, &nf)
}

// apiStatusCode returns the HTTP status from an SDK error, or 0 if the error
// isn't (or doesn't wrap) a CloudStack-backed API error. Statuses the generated
// client doesn't model for a given endpoint surface as a bare *core.APIError, so
// matching on the code is more robust than matching a typed error.
func apiStatusCode(err error) int {
	var ae *core.APIError
	if errors.As(err, &ae) {
		return ae.StatusCode
	}
	return 0
}
