resource "americancloud_isolated_network" "app" {
  name   = "app-net"
  region = "us-west-0"
}

# Allow VMs in the 10.x.x.0/24 slice of the network to reach HTTPS anywhere.
# source_cidr_list is the SOURCE of the outbound traffic (within the network's
# CIDR), not the destination — scope destinations with dest_cidr_list.
resource "americancloud_egress_rule" "https_out" {
  network_id       = americancloud_isolated_network.app.id
  protocol         = "TCP"
  start_port       = 443
  end_port         = 443
  source_cidr_list = americancloud_isolated_network.app.cidr
  dest_cidr_list   = "0.0.0.0/0"
}
