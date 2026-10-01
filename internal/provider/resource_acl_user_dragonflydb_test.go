// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

const dragonflyTestUser = "wepublish-demo-staging"

func testAccDragonflyPreCheck(t *testing.T) {
	if dragonflyAddr == "" {
		t.Fatal("Dragonfly container not started. Ensure TestMain has been called.")
	}
	if err := cleanupDragonflyUsers(context.Background()); err != nil {
		t.Fatalf("failed to clean up Dragonfly users: %v", err)
	}
	if err := cleanupDragonflyKeys(); err != nil {
		t.Fatalf("failed to clean up Dragonfly keys: %v", err)
	}
}

// Write-only attributes need Terraform 1.11 or later.
var writeOnlyVersionChecks = []tfversion.TerraformVersionCheck{
	tfversion.SkipBelow(tfversion.Version1_11_0),
}

func testAccDragonflyProvider() string {
	return fmt.Sprintf(`
provider "redisacl" {
  address  = %q
  password = %q
  acl_save = true
}
`, dragonflyAddr, dragonflyPassword)
}

func testAccDragonflyPrefixUser(password, version string) string {
	return testAccDragonflyProvider() + fmt.Sprintf(`
resource "redisacl_user" "medium" {
  name                = %[1]q
  enabled             = true
  password_wo         = %[2]q
  password_wo_version = %[3]q
  keys                = "~%[1]s:* ~{%[1]s}:*"
  channels            = "&%[1]s:*"
  commands            = "+@all -@dangerous +info"
}
`, dragonflyTestUser, password, version)
}

func TestAccDragonfly_PrefixUserWithoutDrift(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   writeOnlyVersionChecks,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyPrefixUser("first-password", "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("redisacl_user.medium", "password_wo"),
					testAccCheckDragonflyLogin(dragonflyTestUser, "first-password"),
					testAccCheckDragonflyKeyAccess(dragonflyTestUser, "first-password"),
					testAccCheckDragonflyACLSaved(dragonflyTestUser, true),
				),
			},
		},
	})
}

func TestAccDragonfly_RotatesWriteOnlyPasswordInPlace(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   writeOnlyVersionChecks,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyPrefixUser("first-password", "1"),
				Check:  testAccCheckDragonflyLogin(dragonflyTestUser, "first-password"),
			},
			{
				Config: testAccDragonflyPrefixUser("second-password", "2"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("redisacl_user.medium", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDragonflyLogin(dragonflyTestUser, "second-password"),
					testAccCheckDragonflyLoginFails(dragonflyTestUser, "first-password"),
				),
			},
		},
	})
}

func TestAccDragonfly_RecreatesUserDeletedOutsideTerraform(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   writeOnlyVersionChecks,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyPrefixUser("first-password", "1"),
			},
			{
				PreConfig: func() {
					admin := dragonflyAdminClient()
					defer func() { _ = admin.Close() }()
					if err := admin.ACLDelUser(context.Background(), dragonflyTestUser).Err(); err != nil {
						t.Fatalf("failed to delete user outside Terraform: %v", err)
					}
				},
				Config: testAccDragonflyPrefixUser("first-password", "1"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("redisacl_user.medium", plancheck.ResourceActionCreate),
					},
				},
				Check: testAccCheckDragonflyLogin(dragonflyTestUser, "first-password"),
			},
		},
	})
}

func TestAccDragonfly_PasswordFromEnvironment(t *testing.T) {
	t.Setenv("REDIS_PASSWORD", dragonflyPassword)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "redisacl" {
  address = %q
}

resource "redisacl_user" "medium" {
  name      = %q
  enabled   = true
  passwords = ["env-password"]
  keys      = "~%[2]s:*"
  channels  = "&%[2]s:*"
  commands  = "+@all -@dangerous +info"
}
`, dragonflyAddr, dragonflyTestUser),
				Check: testAccCheckDragonflyLogin(dragonflyTestUser, "env-password"),
			},
		},
	})
}

func TestAccDragonfly_RejectsSelectors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyProvider() + fmt.Sprintf(`
resource "redisacl_user" "medium" {
  name      = %q
  passwords = ["pw"]
  selectors = ["~other:* +get"]
}
`, dragonflyTestUser),
				ExpectError: regexp.MustCompile(`(?i)selectors are not supported by Dragonfly`),
			},
		},
	})
}

func testAccCheckDragonflyLogin(username, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client := dragonflyUserClient(username, password)
		defer func() { _ = client.Close() }()

		return client.Ping(context.Background()).Err()
	}
}

func testAccCheckDragonflyLoginFails(username, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client := dragonflyUserClient(username, password)
		defer func() { _ = client.Close() }()

		if err := client.Ping(context.Background()).Err(); err == nil {
			return fmt.Errorf("login as %s with an old password still works", username)
		}

		return nil
	}
}

func testAccCheckDragonflyKeyAccess(username, password string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		ctx := context.Background()
		client := dragonflyUserClient(username, password)
		defer func() { _ = client.Close() }()

		if err := client.Set(ctx, username+"::own", "1", 0).Err(); err != nil {
			return fmt.Errorf("own prefixed key refused: %w", err)
		}
		if err := client.Set(ctx, "{"+username+"}:queue", "1", 0).Err(); err != nil {
			return fmt.Errorf("own hashtag key refused: %w", err)
		}
		if err := client.Set(ctx, "other-medium::key", "1", 0).Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
			return fmt.Errorf("foreign key not refused with NOPERM, got: %v", err)
		}
		if err := client.FlushAll(ctx).Err(); err == nil || !strings.Contains(err.Error(), "NOPERM") {
			return fmt.Errorf("FLUSHALL not refused with NOPERM, got: %v", err)
		}

		return nil
	}
}

func testAccCheckDragonflyACLSaved(username string, present bool) resource.TestCheckFunc {
	return func(*terraform.State) error {
		saved, err := dragonflySavedACL(context.Background())
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(saved), "user "+strings.ToLower(username)+" ") != present {
			return fmt.Errorf("expected user %s present=%t in the aclfile, got:\n%s", username, present, saved)
		}

		return nil
	}
}

func testAccCheckDragonflyUserDestroyed(*terraform.State) error {
	ctx := context.Background()
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()

	users, err := admin.Do(ctx, "ACL", "USERS").StringSlice()
	if err != nil {
		return err
	}
	for _, user := range users {
		if user == dragonflyTestUser {
			return fmt.Errorf("user %s still exists after destroy", dragonflyTestUser)
		}
	}

	return testAccCheckDragonflyACLSaved(dragonflyTestUser, false)(nil)
}

func TestAccDragonfly_UsersDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyProvider() + `
data "redisacl_users" "all" {}
data "redisacl_user" "default" {
  name = "default"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.redisacl_users.all", "users.#"),
					resource.TestCheckResourceAttr("data.redisacl_user.default", "enabled", "true"),
				),
			},
		},
	})
}

func testAccDragonflyDatabaseUser(database string) string {
	return testAccDragonflyProvider() + fmt.Sprintf(`
resource "redisacl_user" "medium" {
  name      = %[1]q
  enabled   = true
  passwords = ["db-password"]
  keys      = "~%[1]s:*"
  channels  = "&%[1]s:*"
  commands  = "+@all -@dangerous +info"
  %[2]s
}
`, dragonflyTestUser, database)
}

func TestAccDragonfly_LocksUserToDatabase(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyDatabaseUser("database = 0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("redisacl_user.medium", "database", "0"),
					testAccCheckDragonflyACLListEndsWith(dragonflyTestUser, "$0"),
					testAccCheckDragonflySelectRefused(dragonflyTestUser, "db-password", 1),
				),
			},
			{
				PreConfig: func() {
					admin := dragonflyAdminClient()
					defer func() { _ = admin.Close() }()
					if err := admin.Do(context.Background(), "ACL", "SETUSER", dragonflyTestUser, "$all").Err(); err != nil {
						t.Fatalf("failed to unlock the database outside Terraform: %v", err)
					}
				},
				Config: testAccDragonflyDatabaseUser("database = 0"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction("redisacl_user.medium", plancheck.ResourceActionUpdate),
					},
				},
				Check: testAccCheckDragonflyACLListEndsWith(dragonflyTestUser, "$0"),
			},
			{
				Config: testAccDragonflyDatabaseUser(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("redisacl_user.medium", "database"),
					testAccCheckDragonflyACLListEndsWith(dragonflyTestUser, "$all"),
				),
			},
		},
	})
}

func TestAccACLUserResource_DatabaseRejectedOnRedis(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "redisacl" {}

resource "redisacl_user" "test" {
  name      = "testuser-db"
  passwords = ["pw"]
  database  = 0
}
`,
				ExpectError: regexp.MustCompile(`(?i)database is only supported by Dragonfly`),
			},
		},
	})
}

func testAccCheckDragonflyACLListEndsWith(username, suffix string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		admin := dragonflyAdminClient()
		defer func() { _ = admin.Close() }()

		lines, err := admin.Do(context.Background(), "ACL", "LIST").StringSlice()
		if err != nil {
			return err
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "user "+username+" ") {
				if !strings.HasSuffix(line, " "+suffix) {
					return fmt.Errorf("expected ACL LIST line to end with %q, got: %s", suffix, line)
				}
				return nil
			}
		}

		return fmt.Errorf("user %s not found in ACL LIST", username)
	}
}

func testAccCheckDragonflySelectRefused(username, password string, db int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		client := dragonflyUserClient(username, password)
		defer func() { _ = client.Close() }()

		err := client.Do(context.Background(), "SELECT", db).Err()
		if err == nil || !strings.Contains(err.Error(), "NOPERM") {
			return fmt.Errorf("SELECT %d was not refused with NOPERM, got: %v", db, err)
		}

		return nil
	}
}

func dragonflyAdminDo(t *testing.T, args ...interface{}) {
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()
	if err := admin.Do(context.Background(), args...).Err(); err != nil {
		t.Fatalf("admin command %v failed: %v", args, err)
	}
}

func TestAccDragonfly_DetectsPasswordChangedOutsideTerraform(t *testing.T) {
	expectUpdate := resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction("redisacl_user.medium", plancheck.ResourceActionUpdate),
		},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		TerraformVersionChecks:   writeOnlyVersionChecks,
		CheckDestroy:             testAccCheckDragonflyUserDestroyed,
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyPrefixUser("managed-password", "1"),
			},
			{
				PreConfig:        func() { dragonflyAdminDo(t, "ACL", "SETUSER", dragonflyTestUser, ">backdoor-password") },
				Config:           testAccDragonflyPrefixUser("managed-password", "1"),
				ConfigPlanChecks: expectUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDragonflyLoginFails(dragonflyTestUser, "backdoor-password"),
					testAccCheckDragonflyLogin(dragonflyTestUser, "managed-password"),
				),
			},
			{
				PreConfig:        func() { dragonflyAdminDo(t, "ACL", "SETUSER", dragonflyTestUser, "resetpass", ">replaced-password") },
				Config:           testAccDragonflyPrefixUser("managed-password", "1"),
				ConfigPlanChecks: expectUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDragonflyLoginFails(dragonflyTestUser, "replaced-password"),
					testAccCheckDragonflyLogin(dragonflyTestUser, "managed-password"),
				),
			},
			{
				PreConfig:        func() { dragonflyAdminDo(t, "ACL", "SETUSER", dragonflyTestUser, "nopass") },
				Config:           testAccDragonflyPrefixUser("managed-password", "1"),
				ConfigPlanChecks: expectUpdate,
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDragonflyLoginFails(dragonflyTestUser, "any-password-at-all"),
					testAccCheckDragonflyLogin(dragonflyTestUser, "managed-password"),
				),
			},
		},
	})
}

func testAccDragonflyUserWithKeyCleanup(deleteKeys string) string {
	return testAccDragonflyProvider() + fmt.Sprintf(`
resource "redisacl_user" "medium" {
  name      = %[1]q
  enabled   = true
  passwords = ["cleanup-password"]
  keys      = "~%[1]s:* ~{%[1]s}:*"
  channels  = "&%[1]s:*"
  commands  = "+@all -@dangerous +info"
  database  = 0
  %[2]s
}
`, dragonflyTestUser, deleteKeys)
}

// seedCleanupKeys writes 2500 of the user's own keys (more than one SCAN batch),
// a BullMQ-style key, and keys that must survive: another tenant's and a
// look-alike prefix.
func seedCleanupKeys(t *testing.T) {
	ctx := context.Background()
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()

	pipe := admin.Pipeline()
	for i := 0; i < 2500; i++ {
		pipe.Set(ctx, fmt.Sprintf("%s::cache:%d", dragonflyTestUser, i), "v", 0)
	}
	pipe.Set(ctx, "{"+dragonflyTestUser+"}:bull:queue:1", "job", 0)
	pipe.Set(ctx, "other-medium::key", "keep", 0)
	pipe.Set(ctx, dragonflyTestUser+"-evil::key", "keep", 0)
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatalf("failed to seed keys: %v", err)
	}
}

func countKeys(pattern string) (int, error) {
	ctx := context.Background()
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()

	count := 0
	iter := admin.Scan(ctx, 0, pattern, 1000).Iterator()
	for iter.Next(ctx) {
		count++
	}

	return count, iter.Err()
}

func expectKeyCounts(expected map[string]int) error {
	for pattern, want := range expected {
		got, err := countKeys(pattern)
		if err != nil {
			return err
		}
		if got != want {
			return fmt.Errorf("expected %d keys matching %q, got %d", want, pattern, got)
		}
	}

	return nil
}

func TestAccDragonfly_DeletesKeysOnDestroy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckDragonflyUserDestroyed(s); err != nil {
				return err
			}
			defer func() { _ = cleanupDragonflyKeys() }()

			return expectKeyCounts(map[string]int{
				dragonflyTestUser + ":*":        0,
				"{" + dragonflyTestUser + "}:*": 0,
				"other-medium::*":               1,
				dragonflyTestUser + "-evil::*":  1,
			})
		},
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyUserWithKeyCleanup(fmt.Sprintf(`delete_keys_on_destroy = ["%[1]s:*", "{%[1]s}:*"]`, dragonflyTestUser)),
				Check: func(*terraform.State) error {
					seedCleanupKeys(t)
					return expectKeyCounts(map[string]int{dragonflyTestUser + ":*": 2500})
				},
			},
		},
	})
}

func TestAccDragonfly_KeepsKeysWithoutCleanupOption(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckDragonflyUserDestroyed(s); err != nil {
				return err
			}
			defer func() { _ = cleanupDragonflyKeys() }()

			return expectKeyCounts(map[string]int{dragonflyTestUser + ":*": 2500})
		},
		Steps: []resource.TestStep{
			{
				Config: testAccDragonflyUserWithKeyCleanup(""),
				Check: func(*terraform.State) error {
					seedCleanupKeys(t)
					return nil
				},
			},
		},
	})
}

func TestAccDragonfly_RejectsBroadDeletePattern(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccDragonflyPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccDragonflyUserWithKeyCleanup(`delete_keys_on_destroy = ["*"]`),
				ExpectError: regexp.MustCompile(`(?i)must start with a fixed prefix`),
			},
		},
	})
}

func cleanupDragonflyKeys() error {
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()

	return admin.FlushAll(context.Background()).Err()
}
