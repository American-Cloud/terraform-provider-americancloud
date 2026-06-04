# DNS records have no server-side id; import by composite id zoneId/name/type/content.
terraform import americancloud_dns_record.www "11111111-1111-1111-1111-111111111111/www/A/203.0.113.10"
