resource "americancloud_isolated_network" "app" {
  name   = "app-net"
  region = "us-west-0"
}

resource "americancloud_public_ip" "app" {
  network_id = americancloud_isolated_network.app.id
  region     = "us-west-0"
}

resource "americancloud_vm" "web" {
  name                = "web-1"
  region              = "us-west-0"
  vm_package          = "standard-custom"
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04"
  network             = americancloud_isolated_network.app.id
  subscription_period = "hourly"
}

# Forward public port 2222 to the VM's SSH port. A matching firewall rule is
# required for the traffic to be allowed — forwarding routes it, the firewall
# admits it.
resource "americancloud_port_forwarding_rule" "ssh" {
  ip_id        = americancloud_public_ip.app.id
  vm_id        = americancloud_vm.web.id
  public_port  = 2222
  private_port = 22
  protocol     = "TCP"
}

resource "americancloud_firewall_rule" "ssh" {
  ip_id            = americancloud_public_ip.app.id
  protocol         = "TCP"
  start_port       = 2222
  end_port         = 2222
  source_cidr_list = "0.0.0.0/0"
  type             = "Ingress"
}
