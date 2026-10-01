---
page_title: "Getting started"
subcategory: ""
description: |-
  Create your first Redis ACL user with Terraform and bring existing users under Terraform.
---

# Getting started

This guide takes you from an empty configuration to an application user on
your Redis server. It also shows how to take over users that already exist.

## Before you start

You need:

- Redis 6.0 or later, or Dragonfly, reachable from where you run Terraform.
- Terraform 1.11 or later, for write-only passwords. Older versions work with
  the `passwords` argument instead; see
  [Managing passwords](https://registry.terraform.io/providers/wepublish/redisacl/latest/docs/guides/managing-passwords).
- A user that Terraform logs in as and that may run `ACL` commands, e.g. your
  admin user. Don't manage that user with the same configuration.

## 1. Configure the provider

Create `main.tf`:

```terraform
terraform {
  required_providers {
    redisacl = {
      source  = "wepublish/redisacl"
      version = "~> 1.1"
    }
  }
}

provider "redisacl" {
  address = "redis.example.com:6379"
}
```

Pass the admin credentials through the environment, so they stay out of your
code:

```shell
export REDIS_USERNAME=admin
export REDIS_PASSWORD='the-admin-password'
```

Add `use_tls = true` when your server uses TLS. The
[provider documentation](https://registry.terraform.io/providers/wepublish/redisacl/latest/docs)
also covers Cluster, Sentinel and mutual TLS.

## 2. Describe a user

Add an application user that may only use keys and channels starting with
`billing:`:

```terraform
variable "billing_app_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "redisacl_user" "billing" {
  name    = "billing-app"
  enabled = true

  password_wo         = var.billing_app_password
  password_wo_version = "1"

  keys     = "~billing:*"
  channels = "&billing:*"
  commands = "+@read +@write -@dangerous"
}
```

`commands` is an allow-list: the provider puts `-@all` in front of it, so the
user may run read and write commands and nothing else. Always set `keys`,
`channels` and `commands`; a user without them gets full access.

## 3. Apply

```shell
terraform init
export TF_VAR_billing_app_password='a-long-random-password'
terraform apply
```

Terraform shows the user it will create. Confirm with `yes`.

## 4. Check the result

Log in as the new user. It may use its own keys:

```shell
redis-cli -h redis.example.com --user billing-app --pass 'a-long-random-password' SET billing:test 1
```

Keys outside its prefix are refused with a `NOPERM` error:

```shell
redis-cli -h redis.example.com --user billing-app --pass 'a-long-random-password' GET other:key
```

As admin, `ACL GETUSER billing-app` shows the rules the provider set.

## 5. Change it

Edit the configuration, e.g. allow a second prefix with
`keys = "~billing:* ~invoices:*"`, then run `terraform apply` again. The user is
updated in place.

If someone changes the user by hand, the next `terraform plan` shows the
difference and `terraform apply` sets it back.

## 6. Bring existing users under Terraform

Write a `redisacl_user` resource for the user, then import it by name
(Terraform 1.5+):

```terraform
import {
  to = redisacl_user.billing
  id = "billing-app"
}
```

Run `terraform plan`. It shows where your configuration differs from the
server; adjust the configuration until only the changes you want are left.
Redis does not reveal passwords, so the first apply sets the password from your
configuration. Use the password your applications already use.

To find users that exist on the server but not in your configuration, see the
[`redisacl_users` data source](https://registry.terraform.io/providers/wepublish/redisacl/latest/docs/data-sources/users).

## Next steps

- [`redisacl_user` resource](https://registry.terraform.io/providers/wepublish/redisacl/latest/docs/resources/user):
  every argument and how each one maps to ACL rules
- [Managing passwords](https://registry.terraform.io/providers/wepublish/redisacl/latest/docs/guides/managing-passwords):
  rotation and changes made on the server
