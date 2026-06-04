default: build

build:
	go build ./...

install:
	go install .

# Unit + SDK-coverage tests (no credentials, no infrastructure).
test:
	go test ./... -timeout 120s

# Acceptance tests create real, billable infrastructure on the target API.
# Export AMERICANCLOUD_API_CLIENT_ID/_SECRET (+ AMERICANCLOUD_API_URL to override the API endpoint).
testacc:
	TF_ACC=1 go test ./... -timeout 120m

fmt:
	gofmt -w .

vet:
	go vet ./...

# Regenerate Registry docs from schemas + examples/.
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate --provider-name americancloud

.PHONY: default build install test testacc fmt vet docs
