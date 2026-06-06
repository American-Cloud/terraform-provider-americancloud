resource "americancloud_isolated_network" "app" {
  name   = "app-net"
  region = "us-west-0"
}

resource "americancloud_public_ip" "app" {
  network_id = americancloud_isolated_network.app.id
  region     = "us-west-0"
}

resource "americancloud_vm" "web" {
  count               = 2
  name                = "web-${count.index}"
  region              = "us-west-0"
  vm_package          = "standard-custom"
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04"
  network             = americancloud_isolated_network.app.id
  subscription_period = "hourly"
}

# Balance public HTTP across the web VMs. name, algorithm, description, and
# instance_ids change in place; ports/protocol/CIDR/IP replace the rule.
resource "americancloud_load_balancer_rule" "web" {
  ip_id        = americancloud_public_ip.app.id
  name         = "web-lb"
  algorithm    = "roundrobin"
  public_port  = 80
  private_port = 8080
  instance_ids = americancloud_vm.web[*].id
}
