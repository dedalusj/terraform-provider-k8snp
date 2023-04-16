package provider

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	apimachineryschema "k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Ensure K8sNpProvider satisfies various provider interfaces.
var _ provider.Provider = &K8sNpProvider{}

// K8sNpProvider defines the provider implementation.
type K8sNpProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// K8sNpProviderModel describes the provider data model.
type K8sNpProviderModel struct {
	KubeHost             types.String `tfsdk:"kube_host"`
	ClusterCaCertificate types.String `tfsdk:"cluster_ca_certificate"`
	Token                types.String `tfsdk:"token"`
}

func (p *K8sNpProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "k8snp"
	resp.Version = p.version
}

func (p *K8sNpProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"kube_host": schema.StringAttribute{
				Required:    true,
				Description: "The hostname (in form of URI) of the Kubernetes API",
				Validators:  []validator.String{HttpsOrigin()},
			},
			"cluster_ca_certificate": schema.StringAttribute{
				Required:    true,
				Description: "PEM-encoded root certificates bundle for TLS authentication.",
			},
			"token": schema.StringAttribute{
				Required:    true,
				Description: "Token to authenticate an service account",
				Sensitive:   true,
			},
		},
	}
}

func (p *K8sNpProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data K8sNpProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.KubeHost.IsUnknown() || data.Token.IsUnknown() || data.ClusterCaCertificate.IsUnknown() {
		return
	}

	parsed, err := url.Parse(data.KubeHost.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("kube_host"),
			"Unknown Kube Host",
			"Invalid format for the k8s host URL: "+err.Error(),
		)
		return
	}

	if parsed.Scheme != "https" {
		resp.Diagnostics.AddAttributeError(
			path.Root("kube_host"),
			"Unknown Kube Host",
			"Invalid format for the k8s host URL. Only HTTPS hosts are allowed",
		)
		return
	}

	config, err := initializeConfiguration(&data, req.TerraformVersion)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create k8s client config",
			"Unexpected error while creating k8s client config: "+err.Error(),
		)
		return
	}

	resp.DataSourceData = config
	resp.ResourceData = config
}

func (p *K8sNpProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewNodePoolResource,
	}
}

func (p *K8sNpProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &K8sNpProvider{
			version: version,
		}
	}
}

func initializeConfiguration(m *K8sNpProviderModel, terraformVersion string) (*restclient.Config, error) {
	overrides := &clientcmd.ConfigOverrides{}
	loader := &clientcmd.ClientConfigLoadingRules{}

	overrides.ClusterInfo.CertificateAuthorityData = bytes.NewBufferString(m.ClusterCaCertificate.ValueString()).Bytes()

	host, _, err := restclient.DefaultServerURL(m.KubeHost.ValueString(), "", apimachineryschema.GroupVersion{}, false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse host: %s", err)
	}
	overrides.ClusterInfo.Server = host.String()

	overrides.AuthInfo.Token = m.Token.ValueString()

	cc := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides)
	cfg, err := cc.ClientConfig()
	if err != nil {
		log.Printf("[WARN] Invalid provider configuration was supplied. Provider operations likely to fail: %v", err)
		return nil, nil
	}

	cfg.UserAgent = fmt.Sprintf("HashiCorp/1.0 Terraform/%s", terraformVersion)

	return cfg, nil
}
