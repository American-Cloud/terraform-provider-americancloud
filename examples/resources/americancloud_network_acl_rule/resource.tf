resource "americancloud_vpc_network" "main" {
  name   = "main-vpc"
  region = "us-west-0"
  cidr   = "10.10.0.0/16"
}

resource "americancloud_network_acl" "web_tier" {
  name   = "web-tier-acl"
  vpc_id = americancloud_vpc_network.main.id
}

# Rules evaluate in ascending number order; first match wins.
resource "americancloud_network_acl_rule" "allow_https" {
  acl_id       = americancloud_network_acl.web_tier.id
  number       = 10
  protocol     = "TCP"
  cidr_list    = "0.0.0.0/0"
  action       = "Allow"
  traffic_type = "Ingress"
  start_port   = 443
  end_port     = 443
}

resource "americancloud_network_acl_rule" "deny_rest" {
  acl_id       = americancloud_network_acl.web_tier.id
  number       = 100
  protocol     = "ALL"
  cidr_list    = "0.0.0.0/0"
  action       = "Deny"
  traffic_type = "Ingress"
}

# Attach the ACL to a tier.
resource "americancloud_vpc_tier" "web" {
  vpc_id  = americancloud_vpc_network.main.id
  name    = "web"
  gateway = "10.10.1.1"
  netmask = "255.255.255.0"
  acl_id  = americancloud_network_acl.web_tier.id
}
