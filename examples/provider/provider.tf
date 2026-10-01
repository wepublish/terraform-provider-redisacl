terraform {
  required_providers {
    redisacl = {
      source  = "wepublish/redisacl"
      version = "~> 1.1"
    }
  }
}

# Terraform logs in as an admin user; its credentials come from the
# REDIS_USERNAME and REDIS_PASSWORD environment variables.
provider "redisacl" {
  address = "redis.example.com:6379"
  use_tls = true
}
