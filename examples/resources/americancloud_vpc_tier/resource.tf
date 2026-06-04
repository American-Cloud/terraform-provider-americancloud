resource "americancloud_vpc_network" "main" {
  name   = "app-vpc"
  region = "us-west-0"
  cidr   = "10.20.0.0/16"
}

resource "americancloud_vpc_tier" "web" {
  vpc_id  = americancloud_vpc_network.main.id
  name    = "web"
  gateway = "10.20.1.1"
  netmask = "255.255.255.0"
}
