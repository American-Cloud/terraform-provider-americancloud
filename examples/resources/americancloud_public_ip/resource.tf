resource "americancloud_isolated_network" "example" {
  name   = "app-net"
  region = "us-west-0"
}

resource "americancloud_public_ip" "example" {
  network_id = americancloud_isolated_network.example.id
  region     = "us-west-0"
}
