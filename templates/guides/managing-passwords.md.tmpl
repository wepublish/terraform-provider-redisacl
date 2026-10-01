---
page_title: "Managing passwords"
subcategory: ""
description: |-
  Choose between write-only passwords and the passwords list, rotate passwords, and handle changes made on the server.
---

# Managing passwords

A `redisacl_user` gets its password in one of two ways. You can't use both on
the same user.

|                                       | `password_wo` (recommended)       | `passwords`                    |
|---------------------------------------|-----------------------------------|--------------------------------|
| Stored in the plan and the state      | Never                             | Yes, marked as sensitive       |
| Terraform version                     | 1.11 or later                     | Any                            |
| Passwords per user                    | One                               | Any number                     |
| Detects a password changed on the server | Yes                            | No                             |
| Rotate                                | Change `password_wo_version`      | Edit the list                  |

## Write-only password

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

The password is sent to Redis but never written to the plan or the state.
Terraform only keeps a SHA-256 hash of it, the same form Redis stores. That's
how the provider notices a password changed on the server.

### Keep the value stable

The provider sends `password_wo` to Redis on **every** create and update of the
user, also when you only change `keys`. The value must therefore be the same on
every run. Read it from your secret store or a CI secret.

!> Don't use a value that changes on every run, such as an ephemeral
`random_password`. Every update of the user would then set a new password and
lock your application out.

### Rotate the password

1. Store the new password where Terraform reads it from.
2. Change `password_wo_version`, e.g. from `"1"` to `"2"`. Any new value works.
3. Run `terraform apply`.

The old password stops working at once. Update your applications at the same
time, or use `passwords` with two values for a rotation without downtime (see
below).

### Changes made on the server

If someone sets a different password, e.g. with `redis-cli`, the next plan
shows `password_wo_version = "changed outside Terraform"`. `terraform apply`
then sets your password again.

## Passwords list

```terraform
resource "redisacl_user" "reporting" {
  name      = "reporting"
  enabled   = true
  passwords = [var.reporting_password]

  keys     = "~stats:*"
  channels = ""
  commands = "+@read -@dangerous"
}
```

Use `passwords` when you need several passwords at once, or Terraform older
than 1.11. The values are stored in the state, so protect your state backend.
Redis does not reveal passwords, so a password changed on the server is not
detected.

!> `passwords = []` doesn't mean "no password set". It means the user needs
**no password** at all (`nopass`): anyone can log in as this user.

### Rotate without downtime

1. Add the new password: `passwords = [var.old_password, var.new_password]`, then apply.
2. Switch your applications to the new password.
3. Remove the old one: `passwords = [var.new_password]`, then apply.

## Switching from `passwords` to `password_wo`

Remove `passwords`, add `password_wo` and `password_wo_version`, and apply. The
provider removes the old passwords and sets the write-only one. Old state
versions, e.g. in your backend's history, still contain the previous values.
Use a new password if that matters to you.
