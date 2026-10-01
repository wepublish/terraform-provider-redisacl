# Terraform 1.5+: import with a block and review the result in the plan.
import {
  to = redisacl_user.billing
  id = "billing-app"
}
