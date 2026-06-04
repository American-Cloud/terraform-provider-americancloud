# A snapshot targets an *attached* volume. In this version the attachable
# volume is a VM's root disk, so snapshot a VM root volume with type "RootDisk".
resource "americancloud_isolated_network" "example" {
  name   = "snap-net"
  region = "us-west-0"
}

resource "americancloud_vm" "example" {
  name                = "snap-source"
  region              = "us-west-0"
  vm_package          = "standard-custom"
  vcpu                = 1
  memory_mb           = 2048
  root_disk_gb        = 25
  image               = "ubuntu-24.04-050826" # from list_images
  network             = americancloud_isolated_network.example.id
  subscription_period = "hourly"
}

resource "americancloud_snapshot" "backup" {
  volume_id = americancloud_vm.example.root_volume_id
  name      = "root-backup"
  type      = "RootDisk"
}
