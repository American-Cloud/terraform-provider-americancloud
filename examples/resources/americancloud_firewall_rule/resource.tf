resource "americancloud_isolated_network" "example" {
  name   = "app-net"
  region = "us-west-0"
}

resource "americancloud_public_ip" "example" {
  network_id = americancloud_isolated_network.example.id
  region     = "us-west-0"
}

# Allow inbound SSH from anywhere
resource "americancloud_firewall_rule" "ssh" {
  ip_id            = americancloud_public_ip.example.id
  protocol         = "TCP"
  start_port       = "22"
  end_port         = "22"
  source_cidr_list = "0.0.0.0/0"
  type             = "Ingress"
}
