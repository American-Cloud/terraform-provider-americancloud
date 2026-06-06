package provider

import "context"

// findInPages pages a List endpoint and returns the first item matching pred, or
// the zero value (nil for a pointer T) when none match. Resources whose API has
// no GET-by-id endpoint do their Read as "list and match" — this is the shared
// loop so each resource only supplies how to fetch a page and how to match.
func findInPages[T any](
	ctx context.Context,
	listPage func(ctx context.Context, page, pageSize int) ([]T, error),
	pred func(T) bool,
) (T, error) {
	const pageSize = 100
	var zero T
	for page := 1; ; page++ {
		items, err := listPage(ctx, page, pageSize)
		if err != nil {
			return zero, err
		}
		for _, item := range items {
			if pred(item) {
				return item, nil
			}
		}
		if len(items) < pageSize {
			return zero, nil // reached the last page without a match
		}
	}
}

// collectPages pages a list endpoint to exhaustion and returns all items —
// the no-predicate sibling of findInPages, for Reads that need the full set
// (e.g. a load balancer rule's applied instances).
func collectPages[T any](
	ctx context.Context,
	listPage func(ctx context.Context, page, pageSize int) ([]T, error),
) ([]T, error) {
	const pageSize = 100
	var all []T
	for page := 1; ; page++ {
		items, err := listPage(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < pageSize {
			return all, nil
		}
	}
}
