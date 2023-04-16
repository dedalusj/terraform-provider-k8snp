terraform {
  required_providers {
    k8snp = {
      source = "registry.terraform.io/dedalusj/k8snp"
    }
    google = {
      source  = "hashicorp/google"
      version = "4.61.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "3.5.1"
    }
  }
}

variable "google_project_id" {
  type = string
}

provider "google" {
  project = var.google_project_id
  region  = "australia-southeast2"
}

provider "k8snp" {
  kube_host              = "https://${google_container_cluster.safe_node_pool.endpoint}"
  cluster_ca_certificate = base64decode(google_container_cluster.safe_node_pool.master_auth.0.cluster_ca_certificate)
  token                  = data.google_client_config.current.access_token
}

data "google_client_config" "current" {}

resource "google_service_account" "safe_node_pool" {
  account_id   = "safe-node-pool-sa"
  display_name = "Service Account used in the safe node pool example project"
}

resource "google_container_cluster" "safe_node_pool" {
  name     = "safe-node-pool-gke-cluster"
  location = "australia-southeast2"

  # We can't create a cluster with no node pool defined, but we want to only use
  # separately managed node pools. So we create the smallest possible default
  # node pool and immediately delete it.
  remove_default_node_pool = true
  initial_node_count       = 1
}

# Trigger a new name for our node pool for every property that
# should generate a new node pool and replace the old one
resource "random_id" "node_pool" {
  keepers = {
    # change this value to generate a new name for the node
    # pool and trigger it's recreation.
    # In practice things like machine type, disk size, scopes
    # etc should be here
    trigger = "trigger"
  }

  byte_length = 4
}

# Create a GKE node pool with the settings needed and
# a create_before_destroy lifecycle setting
resource "google_container_node_pool" "safe_node_pool" {
  name               = "safe-node-pool-${random_id.node_pool.hex}"
  location           = "australia-southeast2"
  cluster            = google_container_cluster.safe_node_pool.name
  initial_node_count = 1

  autoscaling {
    min_node_count = 0
    max_node_count = 1
  }

  node_config {
    machine_type = "n1-standard-1"
    disk_size_gb = 20

    service_account = google_service_account.safe_node_pool.email
    oauth_scopes = [
      "https://www.googleapis.com/auth/cloud-platform"
    ]
  }

  lifecycle {
    create_before_destroy = true
  }
}

# Create the safe node pool with the same name as the
# GKE node pool and the create_before_destroy lifecycle
# setting.
#
# The name of the node pool will be used to trigger the
# cordon and drain operations. It will also be used to
# monitor the number of ready nodes in the pool and
# ensure the resource is not considered created until
# all the expected nodes are ready.
resource "k8snp_pool" "node_pool" {
  node_pool_name  = google_container_node_pool.safe_node_pool.name
  min_ready_nodes = 2
  drain_wait      = "60s"

  lifecycle {
    create_before_destroy = true
  }
}