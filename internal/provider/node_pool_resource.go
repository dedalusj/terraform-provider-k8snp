package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/drain"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NodePoolResource{}
var _ resource.ResourceWithImportState = &NodePoolResource{}

func NewNodePoolResource() resource.Resource {
	return &NodePoolResource{}
}

// NodePoolResource defines the resource implementation.
type NodePoolResource struct {
	config    *restclient.Config
	k8sClient *kubernetes.Clientset
}

// NodePoolResourceModel describes the resource data model.
type NodePoolResourceModel struct {
	NodePoolName      types.String `tfsdk:"node_pool_name"`
	NodeSelectorKey   types.String `tfsdk:"node_selector_key"`
	NodeSelectorValue types.String `tfsdk:"node_selector_value"`
	MinReadyNodes     types.Int64  `tfsdk:"min_ready_nodes"`
	ReadyTimeout      types.String `tfsdk:"ready_timeout"`
	DrainTimeout      types.String `tfsdk:"drain_timeout"`
	DrainWaitTime     types.String `tfsdk:"drain_wait"`
}

func (r *NodePoolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_node_pool"
}

func (r *NodePoolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Node pool",

		Attributes: map[string]schema.Attribute{
			"node_pool_name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Node pool name",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"min_ready_nodes": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Minimum number of ready nodes in the new node pool. Defaults to `1`.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
				Default:    int64default.StaticInt64(1),
				Validators: []validator.Int64{int64validator.AtLeast(1)},
			},
			"node_selector_key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Label key used to select the nodes affected by this resource. Defaults to `cloud.google.com/gke-nodepool`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Default: stringdefault.StaticString("cloud.google.com/gke-nodepool"),
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"node_selector_value": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Label value used to select the nodes affected by this resource. Defaults to the node pool name.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"ready_timeout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Maximum time for waiting for nodes in a new node pool to be ready. Defaults to `300s`.",
				Default:             stringdefault.StaticString("300s"),
				Validators: []validator.String{
					MinDuration(0),
				},
			},
			"drain_timeout": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Timeout for node drain operations. Defaults to `300s`.",
				Default:             stringdefault.StaticString("300s"),
				Validators: []validator.String{
					MinDuration(0),
				},
			},
			"drain_wait": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Amount of time to wait after each node drain operation. Defaults to `60s`.",
				Default:             stringdefault.StaticString("60s"),
				Validators: []validator.String{
					MinDuration(0),
				},
			},
		},
	}
}

func (r *NodePoolResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	var ok bool
	r.config, ok = req.ProviderData.(*restclient.Config)
	if !ok {
		resp.Diagnostics.AddError(
			"Unable to get kubernetes config",
			"Unexpected error while fetching kubernetes config",
		)
		return
	}

	k8sClient, err := kubernetes.NewForConfig(r.config)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create kubernetes client",
			"Unexpected error while creating kubernetes client: "+err.Error(),
		)
		return
	}
	r.k8sClient = k8sClient
}

func (r *NodePoolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data *NodePoolResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("waiting for %d nodes to be ready in node pool %s", data.MinReadyNodes.ValueInt64(), data.NodePoolName.ValueString()))

	// we ignore the error as the validator for the argument in the schema
	// definition above will ensure its validity
	readyTimeout, _ := time.ParseDuration(data.ReadyTimeout.ValueString())

	labelKey := data.NodeSelectorKey.ValueString()
	labelValue := data.NodePoolName.ValueString()
	if !data.NodeSelectorValue.IsUnknown() && !data.NodeSelectorValue.IsNull() {
		labelValue = data.NodeSelectorValue.ValueString()
	}

	deadline := time.Now().Add(readyTimeout)
	for time.Now().Before(deadline) {
		nodes, err := r.listNodes(ctx, labelKey, labelValue)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error creating safe node pool",
				fmt.Sprintf("Could not create safe node pool, unexpected error listing current nodes in pool %s: %s", data.NodePoolName.ValueString(), err.Error()),
			)
			return
		}

		numReadyNodes := countReadyNodes(nodes)
		if numReadyNodes >= data.MinReadyNodes.ValueInt64() {
			tflog.Debug(ctx, fmt.Sprintf("found required number of ready nodes in node pool %s...resource created", data.NodePoolName.ValueString()))

			// Save data into Terraform state
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

			return
		}

		tflog.Debug(ctx, fmt.Sprintf("found %d ready nodes in node pool %s...waiting", numReadyNodes, data.NodePoolName.ValueString()))

		time.Sleep(time.Second)
	}

	resp.Diagnostics.AddError(
		"Error waiting for nodes to be ready",
		fmt.Sprintf("Could not find %d ready nodes in node pool %s in the specified timeout", data.MinReadyNodes.ValueInt64(), data.NodePoolName.ValueString()),
	)

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodePoolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data *NodePoolResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodePoolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data *NodePoolResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NodePoolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data *NodePoolResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("draining node pool %s", data.NodePoolName.ValueString()))

	labelKey := data.NodeSelectorKey.ValueString()
	labelValue := data.NodePoolName.ValueString()
	if !data.NodeSelectorValue.IsUnknown() && !data.NodeSelectorValue.IsNull() {
		labelValue = data.NodeSelectorValue.ValueString()
	}

	nodes, err := r.listNodes(ctx, labelKey, labelValue)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error deleting safe node pool",
			fmt.Sprintf("Could not delete safe node pool, unexpected error listing nodes in pool %s: %s", data.NodePoolName.ValueString(), err.Error()),
		)
		return
	}

	// we ignore the error as the validator for the argument in the schema
	// definition above will ensure its validity
	drainTimeout, _ := time.ParseDuration(data.DrainTimeout.ValueString())
	drainWait, _ := time.ParseDuration(data.DrainWaitTime.ValueString())

	// cordon all the old nodes first so that the pods will not
	// be scheduled on nodes that we are about to delete
	for _, node := range nodes {
		drainer := &drain.Helper{
			Ctx:                 ctx,
			Client:              r.k8sClient,
			IgnoreAllDaemonSets: true,
			DeleteEmptyDirData:  true,
			GracePeriodSeconds:  -1,
			Timeout:             drainTimeout,
			OnPodDeletedOrEvicted: func(pod *v1.Pod, usingEviction bool) {
				tflog.Debug(ctx, fmt.Sprintf("evicted pod %s from node %s", pod.Name, node.Name))
			},
			Out:    drainerWriter{ctx: ctx, nodeName: node.Name},
			ErrOut: drainerWriter{ctx: ctx, nodeName: node.Name, isErrOut: true},
		}

		tflog.Debug(ctx, fmt.Sprintf("cordoning node %s", node.Name))
		if err := drain.RunCordonOrUncordon(drainer, &node, true); err != nil {
			resp.Diagnostics.AddError(
				"Error deleting safe node pool",
				fmt.Sprintf("Could not delete safe node pool, unexpected error cordoning node %s: %s", node.Name, err.Error()),
			)
			return
		}
	}

	// then drain them
	for _, node := range nodes {
		drainer := &drain.Helper{
			Ctx:                 ctx,
			Client:              r.k8sClient,
			IgnoreAllDaemonSets: true,
			DeleteEmptyDirData:  true,
			GracePeriodSeconds:  -1,
			Timeout:             drainTimeout,
			OnPodDeletedOrEvicted: func(pod *v1.Pod, usingEviction bool) {
				tflog.Debug(ctx, fmt.Sprintf("evicted pod %s from node %s", pod.Name, node.Name))
			},
			Out:    drainerWriter{ctx: ctx, nodeName: node.Name},
			ErrOut: drainerWriter{ctx: ctx, nodeName: node.Name, isErrOut: true},
		}

		tflog.Debug(ctx, fmt.Sprintf("draining node %s", node.Name))
		if err := drain.RunNodeDrain(drainer, node.Name); err != nil {
			resp.Diagnostics.AddError(
				"Error deleting safe node pool",
				fmt.Sprintf("Could not delete safe node pool, unexpected error draining node %s: %s", node.Name, err.Error()),
			)
			return
		}

		tflog.Debug(ctx, fmt.Sprintf("sleeping after draining node %s", node.Name))
		time.Sleep(drainWait)
	}

}

type drainerWriter struct {
	ctx      context.Context
	nodeName string
	isErrOut bool
}

func (d drainerWriter) Write(p []byte) (n int, err error) {
	var msg strings.Builder
	msg.WriteString("drainer - ")

	if d.isErrOut {
		msg.WriteString("ERROUT - ")
	}

	msg.WriteString("node: " + d.nodeName + " - ")

	msg.Write(p)

	tflog.Debug(d.ctx, msg.String())

	return len(p), nil
}

func (r *NodePoolResource) ImportState(_ context.Context, _ resource.ImportStateRequest, _ *resource.ImportStateResponse) {
}

func (r *NodePoolResource) listNodes(ctx context.Context, labelKey, labelValue string) ([]v1.Node, error) {
	nodeList, err := r.k8sClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", labelKey, labelValue),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return nodeList.Items, nil
}

func countReadyNodes(nodes []v1.Node) int64 {
	var numReadyNodes int64
	for _, node := range nodes {
		for _, condition := range node.Status.Conditions {
			if condition.Type == v1.NodeReady {
				if condition.Status == v1.ConditionTrue {
					numReadyNodes += 1
				}
				break
			}
		}
	}
	return numReadyNodes
}
