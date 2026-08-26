package provider

import (
	"errors"
	"fmt"
	"strings"

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

// deleteBlockedBySnapshots reports whether err is the 409 that refuses a delete
// while the disk still has snapshots, and returns a message naming them. The
// API sends the list in the error body, so the user does not have to look the
// snapshots up. It returns false for every other error.
func deleteBlockedBySnapshots(err error) (string, bool) {
	var ce *acsdk.ConflictError
	if !errors.As(err, &ce) || ce.Body == nil || ce.Body.Code == nil {
		return "", false
	}
	if *ce.Body.Code != "volume_has_snapshots" {
		return "", false
	}
	named := make([]string, 0, len(ce.Body.Snapshots))
	for _, s := range ce.Body.Snapshots {
		if s == nil {
			continue
		}
		// A snapshot the platform left unnamed is still worth reporting by id.
		switch name, id := strings.TrimSpace(s.Name), strings.TrimSpace(s.ID); {
		case name != "" && id != "":
			named = append(named, fmt.Sprintf("%s (%s)", name, id))
		case id != "":
			named = append(named, id)
		case name != "":
			named = append(named, name)
		}
	}
	if len(named) == 0 {
		return "", false
	}
	return fmt.Sprintf(
		"The disk still has snapshots, so it cannot be deleted. Delete these first, then retry: %s.",
		strings.Join(named, ", "),
	), true
}
