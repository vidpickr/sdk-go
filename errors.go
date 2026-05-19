package vidpickr

import "fmt"

// APIError mirrors the {"error": {"code", "message"}} JSON the API
// returns on 4xx/5xx. Branch on Code rather than Message for stable
// behaviour across API versions.
type APIError struct {
	Code       string
	Message    string
	Status     int
	RetryAfter int // seconds; 0 when not provided
}

func (e *APIError) Error() string {
	return fmt.Sprintf("vidpickr api: %s (%s, status %d)", e.Message, e.Code, e.Status)
}

// NoFormatError is returned when the caller requested a quality / codec
// combination that doesn't exist in the /info response.
type NoFormatError struct{ Reason string }

func (e *NoFormatError) Error() string { return "vidpickr: no matching format — " + e.Reason }

// MuxError wraps a failure from the local MP4 mux step.
type MuxError struct{ Reason string }

func (e *MuxError) Error() string { return "vidpickr: mux failed — " + e.Reason }
