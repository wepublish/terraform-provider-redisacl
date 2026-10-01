# Read a user that Terraform does not manage, e.g. the built-in default user.
data "redisacl_user" "default" {
  name = "default"
}

# Terraform 1.5+: warn on every plan while the default user still accepts
# connections.
check "default_user_disabled" {
  assert {
    condition     = !data.redisacl_user.default.enabled
    error_message = "The default user is enabled; disable it once all clients use their own users."
  }
}
