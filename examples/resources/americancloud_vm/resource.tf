resource "americancloud_isolated_network" "example" {
  name   = "vm-net"
  region = "us-west-0"
}

resource "americancloud_ssh_key" "example" {
  name       = "deploy-key"
  public_key = file("~/.ssh/id_ed25519.pub")
}

# VM on a Terraform-managed network.
resource "americancloud_vm" "example" {
  name                = "web-1"
  region              = "us-west-0"
  vm_package          = "standard-custom" # from list_vm_packages
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04" # from list_images
  network             = americancloud_isolated_network.example.id
  subscription_period = "hourly"

  keypairs  = [americancloud_ssh_key.example.name]
  user_data = <<-EOT
    #cloud-config
    packages: [nginx]
  EOT
  tags      = ["web", "production"]
}

# VM with platform-managed network access: omit `network` and the platform
# auto-creates an isolated network, opening the requested inbound ports
# (port forwarding + firewall on the network's public IP).
resource "americancloud_vm" "reachable" {
  name                = "web-2"
  region              = "us-west-0"
  vm_package          = "standard-custom"
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04"
  subscription_period = "hourly"

  keypairs = [americancloud_ssh_key.example.name]

  network_access = {
    allow_egress_all = true
    inbound_ports = [
      { port = 22, protocol = "TCP" },
      { port = 443, protocol = "TCP" },
    ]
  }
}
