package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type durationValidator struct {
	minDuration *time.Duration
	maxDuration *time.Duration
}

func (v durationValidator) Description(_ context.Context) string {
	return "string must be a valid duration format e.g. 60s"
}

func (v durationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v durationValidator) ValidateString(_ context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	parsed, err := time.ParseDuration(value)

	if err != nil {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s is not a duration, got: %s", request.Path, value),
			),
		)
		return
	}

	if v.minDuration != nil && parsed < *v.minDuration {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s is smaller than minimum allowed duration %s, got: %s", request.Path, v.minDuration.String(), value),
			),
		)
		return
	}

	if v.maxDuration != nil && parsed > *v.maxDuration {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s is greater than maximum allowed duration %s, got: %s", request.Path, v.minDuration.String(), value),
			),
		)
		return
	}
}

// Duration returns a validator which ensures the provided value
// is a valid golang time.Duration format, e.g. 60s.
func Duration() validator.String {
	return durationValidator{}
}

// MinDuration returns a validator which ensures the provided value
// is a valid golang time.Duration format, e.g. 60s, and it is
// greater than the provided minimum.
func MinDuration(min time.Duration) validator.String {
	return durationValidator{minDuration: &min}
}

// MaxDuration returns a validator which ensures the provided value
// is a valid golang time.Duration format, e.g. 60s, and it is
// greater than the provided minimum.
func MaxDuration(max time.Duration) validator.String {
	return durationValidator{maxDuration: &max}
}

// DurationInRange returns a validator which ensures the provided value
// is a valid golang time.Duration format, e.g. 60s, and it is
// in the requested range.
func DurationInRange(min, max time.Duration) validator.String {
	return durationValidator{minDuration: &min, maxDuration: &max}
}
