data "redisacl_users" "all" {}

locals {
  managed_users = ["default", "billing-app", "reporting", "tenant-a"]
}

# Users that exist on the server but are not part of this configuration.
output "unmanaged_users" {
  value = setsubtract([for user in data.redisacl_users.all.users : user.name], local.managed_users)
}
