package elasticsearch

import (
	"net/http"
	"testing"
)

func TestIsMissingPITResponse(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		want    bool
	}{
		{"expired search context", http.StatusNotFound, `{"type":"search_context_missing_exception"}`, true},
		{"missing PIT", http.StatusNotFound, `{"type":"point_in_time_missing_exception"}`, true},
		{"missing context text", http.StatusNotFound, "No search context found for id", true},
		{"other not found", http.StatusNotFound, "index_not_found_exception", false},
		{"server error", http.StatusInternalServerError, "search_context_missing_exception", false},
		{"timeout response", http.StatusRequestTimeout, "search_context_missing_exception", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingPITResponse(tt.status, tt.message); got != tt.want {
				t.Fatalf("isMissingPITResponse() = %v, want %v", got, tt.want)
			}
		})
	}
}
