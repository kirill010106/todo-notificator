package apiresponse

import (
	"net/http"
	"strings"
)

// IsWebhook retains the legacy path policy. Webhooks have an empty/text contract
// rather than the application's JSON error contract.
func IsWebhook(r *http.Request) bool {
	return strings.Contains(r.URL.Path, "/webhooks/")
}
