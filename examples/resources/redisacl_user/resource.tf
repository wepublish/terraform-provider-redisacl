# An application that may read and write its own keys. The password is sent
# to Redis but never written to the Terraform state (Terraform 1.11+).
resource "redisacl_user" "billing" {
  name    = "billing-app"
  enabled = true

  password_wo         = var.billing_app_password
  password_wo_version = "1" # change to apply a new password

  keys     = "~billing:*"
  channels = "&billing:*"
  commands = "+@read +@write -@dangerous"
}

# A read-only user. Values in `passwords` are stored in the state; listing
# two of them lets you rotate without downtime.
resource "redisacl_user" "reporting" {
  name      = "reporting"
  enabled   = true
  passwords = [var.reporting_password, var.reporting_password_next]

  keys     = "~billing:* ~stats:*"
  channels = ""
  commands = "+@read -@dangerous"
}

# A tenant on a shared server. Destroying the user also deletes its keys.
resource "redisacl_user" "tenant_a" {
  name    = "tenant-a"
  enabled = true

  password_wo         = var.tenant_a_password
  password_wo_version = "1"

  keys     = "~tenant-a:*"
  channels = "&tenant-a:*"
  commands = "+@all -@dangerous -@admin"

  delete_keys_on_destroy = ["tenant-a:*"]
}
