# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.1.1] - 2026-10-01

### Added
- Guides on the Terraform Registry: getting started, managing passwords, and migrating from
  `B3ns44d/redisacl`
- Documentation of how `redisacl_user` arguments map to ACL rules, including the full-access
  defaults when `keys`, `channels` or `commands` are left out, and focused examples per page

### Fixed
- Documentation and examples configured `cluster` and `sentinel` as blocks (`cluster { ... }`),
  which Terraform rejects; they are attributes (`cluster = { ... }`)

## [1.1.0] - 2026-10-01

### Added
- Dragonfly support, detected automatically from `INFO server`; tested against Dragonfly v2.0.0
- `password_wo` / `password_wo_version` on `redisacl_user`: a write-only password that is never
  stored in state (Terraform 1.11+); changing the version rotates it with an in-place update
- Password drift detection for `password_wo`: the provider keeps the password's SHA-256 in private
  state and compares it with the server's hashes, so an extra, replaced or `nopass` password set
  outside Terraform shows up as a plan change (`password_wo_version = "changed outside Terraform"`)
- `delete_keys_on_destroy` on `redisacl_user`: key patterns deleted (SCAN + UNLINK in batches) right
  after the user is deleted, e.g. a tenant's own prefixes; patterns must start with a fixed prefix
- Provider argument `acl_save` to run `ACL SAVE` after every create, update and delete, so users
  survive a restart of servers that use an aclfile
- `database` on `redisacl_user` (Dragonfly only): restricts the user to one logical database with
  the `$<n>` rule, read back from `ACL LIST` so changes outside Terraform show up in the plan
- `REDIS_ADDRESS`, `REDIS_USERNAME`, `REDIS_PASSWORD` and `REDIS_USE_TLS` environment variables,
  which were documented but not read

### Changed
- The provider is now published as `wepublish/redisacl` (previously `B3ns44d/redisacl`). Change the
  `source` in `required_providers` and migrate existing state with
  `terraform state replace-provider registry.terraform.io/B3ns44d/redisacl registry.terraform.io/wepublish/redisacl`
- `REDIS_URL` is now only a fallback: an `address`, `cluster` or `sentinel` in the provider
  configuration takes precedence (previously the environment variable overrode the configuration)

### Fixed
- Dragonfly rejected every `ACL SETUSER` because of the `reset` rule; the provider now sends
  `resetpass resetkeys resetchannels -@all` there
- Permanent plan diff on Dragonfly, which reports channels as `resetchannels &pattern`
- Self-mutation protection did not work on Dragonfly, which answers `ACL WHOAMI` with `User is <name>`
- `redisacl_user` and `redisacl_users` data sources failed on Dragonfly (flat `ACL GETUSER`
  reply without a `selectors` field)
- Selectors on Dragonfly now fail with a clear error instead of being sent to the server

## [1.0.2] - 2025-11-07

### Fixed
- Fixed Terraform state drift when commands attribute doesn't include `-@all` prefix
  - Provider now intelligently handles `-@all` prefix to prevent unnecessary updates
  - State comparison logic updated to recognize equivalent command configurations
- Improved ACL command rule building to avoid duplicate `-@all` prefixes

### Added
- Comprehensive test suite with HIGH and MEDIUM priority tests
  - **Unit Tests**: 13 test cases for helper functions covering:
    - ACL rule building with various parameter combinations
    - Commands with/without `-@all` prefix handling
    - Multiple passwords, keys, and channel patterns
    - Selectors and complex command combinations
    - Edge cases (null values, empty strings)
  - **Acceptance Tests**: 10 new integration tests covering:
    - Commands drift detection (with/without `-@all` prefix)
    - Multiple password management and rotation
    - Disabled user state management
    - Nopass user handling
    - Complex command combinations (@read, @write, etc.)
    - Multiple key and channel patterns
    - State drift detection and correction
- Helper function `ModifyUserInRedis()` for drift testing

### Changed
- Updated version in GNUmakefile to 1.0.2
- Enhanced test coverage for critical provider functionality

### Documentation
- Added important limitation notice about Redis ACL replication in Sentinel setups
- Clarified that ACL users are not automatically replicated to replica nodes during failover
- Recommended using Redis Cluster for high-availability scenarios requiring ACL persistence

## [1.0.1] - 2025-11-06

### Added
- Generated comprehensive documentation for Terraform Registry
- Provider, resource, and data source documentation with examples
- Auto-generated schema documentation using terraform-plugin-docs

### Fixed
- GoReleaser checksum file naming to follow Terraform Registry requirements
- GPG signing configuration for release artifacts
- Workflow improvements for better release reliability

### Changed
- Removed post-release automation tasks for cleaner release process
- Updated release workflow to handle proper checksum verification

## [1.0.0] - 2025-11-04

### Added

**Core Provider Features:**
- Initial release of the Redis ACL Terraform Provider
- Support for managing Redis ACL users with comprehensive configuration options
- Data sources for reading Redis ACL user and users information
- Full Terraform lifecycle management (Create, Read, Update, Delete, Import)

**Resources:**
- `redisacl_user` - Manage Redis ACL users with support for:
  - User enable/disable state
  - Key patterns and access controls
  - Channel patterns for pub/sub access
  - Command restrictions and permissions
  - Password management (single and multiple passwords)
  - Selector-based permissions (where supported)

**Data Sources:**
- `redisacl_user` - Read information about a specific Redis ACL user
- `redisacl_users` - Read information about all Redis ACL users

**Provider Configuration:**
- Flexible Redis connection options (address, username, password, database)
- TLS support with certificate validation options
- Connection pooling and timeout configuration
- Support for Redis 6.0+ ACL features

**Testing & Quality:**
- Comprehensive unit test suite with 18.3% coverage
- Full acceptance test suite with 59.1% coverage using testcontainers
- Automated integration testing with Redis 6.2, 7.0, and 7.2
- golangci-lint integration with minimal, high-value linter set
- Cross-platform builds (Linux, macOS, Windows on amd64/arm64)

**Documentation:**
- Complete provider documentation with examples
- Resource and data source reference documentation
- Usage examples for common scenarios
- Terraform Registry integration

**CI/CD & Release:**
- GitHub Actions workflows for continuous integration
- Automated testing across multiple Go and Terraform versions
- GoReleaser configuration for multi-platform releases
- GPG signing for security and Terraform Registry compliance
- Automated Terraform Registry publishing

### Technical Details

**Supported Platforms:**
- Linux (amd64, 386, arm, arm64)
- macOS (amd64, arm64)
- Windows (amd64, 386, arm64)
- FreeBSD (amd64, 386, arm, arm64)

**Compatibility:**
- Terraform >= 1.0
- Go 1.23+
- Redis 6.0+ (ACL support required)

**Dependencies:**
- terraform-plugin-framework v1.x
- go-redis/v9 for Redis connectivity
- testcontainers-go for integration testing

### Examples

Basic usage:
```hcl
terraform {
  required_providers {
    redisacl = {
      source  = "B3ns44d/redisacl"
      version = "~> 1.0.0"
    }
  }
}

provider "redisacl" {
  address = "localhost:6379"
}

resource "redisacl_user" "app_user" {
  name     = "app_user"
  enabled  = true
  keys     = "~app:*"
  channels = "&notifications:*"
  commands = "-@all +get +set +del"
  passwords = ["secure_password"]
}
```

## [0.1.0] - Development

### Added
- Initial project structure and development setup
