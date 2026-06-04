package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// pollInterval is how often async resource state is re-checked. The overall
// bound is the operation's context deadline (the resource's create/delete
// timeout), not a fixed attempt count.
const pollInterval = 6 * time.Second

// pollUntil calls check on an interval until it reports done, returns an error,
// or ctx is cancelled (i.e. the Terraform operation timeout elapses). It fires
// once immediately, then every pollInterval.
//
// CRITICAL: callers must persist the resource id into state BEFORE invoking
// pollUntil, so a timeout here never orphans a resource the API already created.
func pollUntil(ctx context.Context, check func(context.Context) (done bool, err error)) error {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			done, err := check(ctx)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
			timer.Reset(pollInterval)
		}
	}
}

// pollReady polls get until status is in ready (success), in failed (an explicit
// failure state → error), or the context deadline elapses.
func pollReady(ctx context.Context, get func(context.Context) (status string, err error), ready, failed []string) error {
	return pollUntil(ctx, func(ctx context.Context) (bool, error) {
		s, err := get(ctx)
		if err != nil {
			return false, err
		}
		if containsFold(ready, s) {
			return true, nil
		}
		if containsFold(failed, s) {
			return false, fmt.Errorf("entered status %q", s)
		}
		return false, nil
	})
}

// transientDeleteTimeout bounds retryTransientDelete: it covers the window in
// which a just-deleted dependent is still releasing its hold on the resource,
// not the resource's own provisioning time.
const transientDeleteTimeout = 10 * time.Minute

// retryTransientDelete calls del until it succeeds, the resource is already
// gone (404), a non-retryable error occurs, or transientDeleteTimeout elapses.
//
// CloudStack releases a just-deleted dependent's hold a beat after the
// dependent's record is gone (a VM's NIC on its network or tier, a tier on its
// VPC, a snapshot's volume mid-expunge), and deleting the parent in the same
// destroy races that: the API returns 409 (resource still has attachments) or
// 504 (release still in progress) until the hold clears. Both are documented
// transient responses (API 1.3.1), so retry — otherwise a `terraform destroy`
// of a VM + its network flakes on the ordering race.
func retryTransientDelete(ctx context.Context, del func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, transientDeleteTimeout)
	defer cancel()
	var lastErr error
	err := pollUntil(ctx, func(ctx context.Context) (bool, error) {
		err := del(ctx)
		switch code := apiStatusCode(err); {
		case err == nil, isNotFound(err):
			return true, nil // deleted, or already gone
		case code == 409, code == 504:
			lastErr = err
			return false, nil // dependents still releasing — keep retrying
		default:
			return false, err
		}
	})
	// On timeout, surface the API's own message, not "context deadline
	// exceeded": a 409 that never clears (e.g. an out-of-band attachment like
	// an associated public IP) is only diagnosable from the underlying error.
	if errors.Is(err, context.DeadlineExceeded) && lastErr != nil {
		return fmt.Errorf("timed out retrying delete; last error: %w", lastErr)
	}
	return err
}

// pollGone polls get until the resource is gone (get reports gone=true, e.g. a
// 404) or its status is one of goneStatuses.
func pollGone(ctx context.Context, get func(context.Context) (status string, gone bool, err error), goneStatuses []string) error {
	return pollUntil(ctx, func(ctx context.Context) (bool, error) {
		s, gone, err := get(ctx)
		if err != nil {
			return false, err
		}
		return gone || containsFold(goneStatuses, s), nil
	})
}

func containsFold(set []string, s string) bool {
	for _, v := range set {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
