# Terraform Provider k8s Node Pool

This plugin allows Terraform users to manage node pools and node groups without downtime.

When managing k8s node pools and node groups with Kubernetes certain managed k8s providers do not properly cordon and drain the nodes before a pool update. This can cause downtime experienced by applications running on those nodes. See [https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/794](https://github.com/terraform-google-modules/terraform-google-kubernetes-engine/issues/794) for an example of the issue.

There are various approaches to solve this problem but none was satisfactory:
- The Terraform destroy provisioners don't run on a resource `create_before_destroy` lifecycle and cannot be used to properly drain a node pool.
- The [baozuo/terraform-google-gke-node-pool](https://github.com/baozuo/terraform-google-gke-node-pool) module does not solve the problem either since the `null_resource` used there does not specify the `create_before_destroy` lifecycle, and it is destroyed before the new node pool is created, see [https://github.com/baozuo/terraform-google-gke-node-pool/issues/1](https://github.com/baozuo/terraform-google-gke-node-pool/issues/1).
- The Terraform [shell provider](https://registry.terraform.io/providers/scottwinkler/shell/1.7.10) does not result in the shell resource being recreated, so it does not receive the old node pool name on recreation and cannot be used to drain safely.

This provider defines a new `node_pool` resources that be used in conjunction with other k8s node pool resources, e.g. [google_container_node_pool](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/container_node_pool) to safely cordon and drain k8s managed node pools.

Specifically when a node pool is created the resource will wait for a specified amount of nodes to be ready and when a node pool is destroyed it will cordon all nodes and safely drain the pods from them one by one.

See [example/resources/k8snp_node_pool/resource.tf](example/resources/k8snp_node_pool/resource.tf) for an example used with a GKE managed node pool.
