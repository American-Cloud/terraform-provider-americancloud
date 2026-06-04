resource "americancloud_kubernetes_cluster" "example" {
  name          = "prod-cluster"
  region        = "us-west-0"
  package       = "basic"  # from list_kubernetes_packages
  version       = "1.33.1" # from list_kubernetes_versions
  control_nodes = 1
  worker_nodes  = 2
}

output "kubeconfig" {
  value     = americancloud_kubernetes_cluster.example.kubeconfig
  sensitive = true
}
