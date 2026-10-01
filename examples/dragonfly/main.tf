# One shared Dragonfly for several tenants: each tenant gets a user that may
# only touch keys and channels starting with its own name.

terraform {
  required_providers {
    redisacl = {
      source = "wepublish/redisacl"
    }
  }
}

# The admin password comes from REDIS_PASSWORD.
provider "redisacl" {
  address     = "dragonfly.example.com:6379"
  use_tls     = true
  tls_ca_cert = file(var.ca_cert_path)
  acl_save    = true
}

variable "ca_cert_path" {
  type = string
}

variable "tenant_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "redisacl_user" "tenant" {
  name = "tenant-a"

  # Never stored in state; bump the version to rotate the password in place.
  password_wo         = var.tenant_password
  password_wo_version = "1"

  keys     = "~tenant-a:* ~{tenant-a}:*"
  channels = "&tenant-a:*"
  commands = "+@all -@dangerous -@admin +info -client -script -function -memory -pubsub -scan -randomkey -dbsize"
  database = 0

  # Delete the tenant's data together with its user.
  delete_keys_on_destroy = ["tenant-a:*", "{tenant-a}:*"]
}
