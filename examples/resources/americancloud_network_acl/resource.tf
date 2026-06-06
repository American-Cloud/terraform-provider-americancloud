resource "americancloud_vpc_network" "main" {
  name   = "main-vpc"
  region = "us-west-0"
  cidr   = "10.10.0.0/16"
}

# Traffic policy for VPC tiers. Attach it via americancloud_vpc_tier.acl_id;
# add rules with americancloud_network_acl_rule.
resource "americancloud_network_acl" "web_tier" {
  name        = "web-tier-acl"
  description = "Inbound rules for the public-facing web tier."
  vpc_id      = americancloud_vpc_network.main.id
}
