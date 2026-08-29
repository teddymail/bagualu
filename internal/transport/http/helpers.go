package httptransport

import (
	"encoding/json"
	"net/http"
)

// decodeJSON decodes the request body into v. Returns an error if the JSON is
// malformed. The caller is responsible for validating required fields.
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}
