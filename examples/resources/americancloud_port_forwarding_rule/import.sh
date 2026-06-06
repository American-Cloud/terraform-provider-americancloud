# Port forwarding rules have no GET endpoint; import by composite id ipId/ruleId.
# open_firewall is create-only and not recoverable on import.
terraform import americancloud_port_forwarding_rule.ssh "feaf8347-9c08-491b-b7cd-0f41fc98530b/b42e1d21-f3c5-4e1f-99ea-9cdf8c26cbf3"
