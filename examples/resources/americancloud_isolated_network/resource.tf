resource "americancloud_isolated_network" "example" {
  name        = "app-net"
  description = "Application tier network"
  region      = "us-west-0"
}
