package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type durationValidator struct{}

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

	if _, err := time.ParseDuration(value); err != nil {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s is not a duration, got: %s", request.Path, value),
			),
		)
		return
	}
}

// Duration returns a validator which ensures the provided value
// is a valid golang time.Duration format, e.g. 60s
func Duration() validator.String {
	return durationValidator{}
}
