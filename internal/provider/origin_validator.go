package provider

import (
	"context"
	"fmt"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type originValidator struct {
	allowedSchemes []string
}

func (v originValidator) Description(_ context.Context) string {
	return "string must be a valid origin with format <scheme>://<host>:<port>"
}

func (v originValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v originValidator) ValidateString(_ context.Context, request validator.StringRequest, response *validator.StringResponse) {
	if request.ConfigValue.IsNull() || request.ConfigValue.IsUnknown() {
		return
	}

	value := request.ConfigValue.ValueString()
	parsed, err := url.Parse(value)
	if err != nil {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s is not a valid origin, got: %s", request.Path, value),
			),
		)
		return
	}

	if parsed.Path != "" {
		response.Diagnostics.Append(
			diag.NewAttributeErrorDiagnostic(
				request.Path,
				"Invalid Attribute Format",
				fmt.Sprintf("Attribute %s does not allow paths, got: %s", request.Path, value),
			),
		)
		return
	}

	for _, allowedScheme := range v.allowedSchemes {
		if parsed.Scheme == allowedScheme {
			return
		}
	}

	response.Diagnostics.Append(
		diag.NewAttributeErrorDiagnostic(
			request.Path,
			"Invalid Attribute Format",
			fmt.Sprintf("Attribute %s has non allowed scheme %s, got: %s", request.Path, parsed.Scheme, value),
		),
	)
}

// Origin returns a validator which ensures that any configured
// attribute value is a valid origin, i.e. in the form <scheme>://<host>:<port>.
func Origin(allowedSchemes []string) validator.String {
	return originValidator{
		allowedSchemes: allowedSchemes,
	}
}

// HttpsOrigin returns a validator which ensure an origins of
// the form https://<host>:<port>
func HttpsOrigin() validator.String {
	return Origin([]string{"https"})
}
