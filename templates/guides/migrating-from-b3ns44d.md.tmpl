---
page_title: "Migrating from B3ns44d/redisacl"
subcategory: ""
description: |-
  Switch existing configurations and state from B3ns44d/redisacl to wepublish/redisacl.
---

# Migrating from B3ns44d/redisacl

This provider was first published as `B3ns44d/redisacl`. From version 1.1.0 on,
it is maintained and published as `wepublish/redisacl`. Resources, data sources
and arguments have the same names, so your configuration only needs a new
source. Your state needs to point to the new provider.

Do the following in every root module that uses the provider.

## 1. Change the source

```terraform
terraform {
  required_providers {
    redisacl = {
      source  = "wepublish/redisacl"
      version = "~> 1.1"
    }
  }
}
```

## 2. Install the new provider

```shell
terraform init
```

Terraform installs `wepublish/redisacl`. It still installs the old provider too,
because the state refers to it.

## 3. Move the state to the new provider

```shell
terraform state replace-provider registry.terraform.io/b3ns44d/redisacl registry.terraform.io/wepublish/redisacl
```

Terraform lists the resources it will change and asks for confirmation. Run
`terraform providers` to check which provider addresses your state uses.

## 4. Check the plan

```shell
terraform plan
```

The plan should show no changes. If it does, check the
[changes in 1.1.0](https://github.com/wepublish/terraform-provider-redisacl/blob/master/CHANGELOG.md):

- `REDIS_URL` is now only used when the provider configuration sets neither
  `address`, `cluster` nor `sentinel`. Before, it overrode the configuration.
- `REDIS_ADDRESS`, `REDIS_USERNAME`, `REDIS_PASSWORD` and `REDIS_USE_TLS` are
  now read. Before, they were documented but ignored. If they are set in your
  environment, they now apply wherever the configuration leaves the matching
  argument out.

Once the plan is clean, commit the configuration change and the new
`.terraform.lock.hcl`.
