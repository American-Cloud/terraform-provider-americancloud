resource "americancloud_dns_zone" "example" {
  name = "example.com"
}

# A record
resource "americancloud_dns_record" "www" {
  zone_id = americancloud_dns_zone.example.id
  name    = "www"
  type    = "A"
  content = "203.0.113.10"
  ttl     = 3600
}

# MX record (priority required)
resource "americancloud_dns_record" "mx" {
  zone_id  = americancloud_dns_zone.example.id
  name     = "@"
  type     = "MX"
  content  = "mail.example.com"
  priority = 10
}
