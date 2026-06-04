resource "americancloud_vpc_network" "example" {
  name   = "prod-vpc"
  region = "us-west-0"
  cidr   = "10.0.0.0/16"
}
