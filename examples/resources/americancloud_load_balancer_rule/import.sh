# Load balancer rules have no GET endpoint; import by composite id ipId/ruleId.
# description is not echoed by the platform and is not recoverable on import.
terraform import americancloud_load_balancer_rule.web "feaf8347-9c08-491b-b7cd-0f41fc98530b/b42e1d21-f3c5-4e1f-99ea-9cdf8c26cbf3"
