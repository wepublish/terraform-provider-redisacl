// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type aclRuleOptions struct {
	// dragonfly is set when the server is Dragonfly, which rejects the
	// "reset" rule (and "clearselectors"), so its parts are spelled out.
	dragonfly bool
	// writeOnlyPassword is the password_wo value from the configuration.
	writeOnlyPassword string
}

func buildACLSetUserRules(data *ACLUserResourceModel, opts aclRuleOptions) []string {
	rules := []string{"reset"}
	if opts.dragonfly {
		rules = []string{"resetpass", "resetkeys", "resetchannels", "-@all"}
	}

	if data.Enabled.IsNull() {
		rules = append(rules, "on")
	} else {
		if data.Enabled.ValueBool() {
			rules = append(rules, "on")
		} else {
			rules = append(rules, "off")
		}
	}

	if opts.writeOnlyPassword != "" {
		rules = append(rules, "resetpass", ">"+opts.writeOnlyPassword)
	} else if !data.Passwords.IsNull() {
		rules = append(rules, "resetpass")
		if len(data.Passwords.Elements()) == 0 {
			rules = append(rules, "nopass")
		} else {
			for _, password := range data.Passwords.Elements() {
				rules = append(rules, ">"+password.(types.String).ValueString())
			}
		}
	}

	if !data.Keys.IsNull() {
		rules = append(rules, "resetkeys")
		rules = append(rules, strings.Fields(data.Keys.ValueString())...)
	} else {
		rules = append(rules, "~*")
	}

	if !data.Channels.IsNull() {
		rules = append(rules, "resetchannels")
		rules = append(rules, strings.Fields(data.Channels.ValueString())...)
	} else {
		rules = append(rules, "&*")
	}

	if !data.Commands.IsNull() {
		commands := data.Commands.ValueString()
		if !strings.HasPrefix(commands, "-@all") {
			rules = append(rules, "-@all")
		}
		rules = append(rules, strings.Fields(commands)...)
	} else {
		rules = append(rules, "+@all")
	}

	if !data.Selectors.IsNull() {
		for _, selector := range data.Selectors.Elements() {
			rules = append(rules, "("+selector.(types.String).ValueString()+")")
		}
	}

	// Dragonfly accepts a single database rule per ACL SETUSER; "$all" also
	// lifts a restriction that was set before.
	if opts.dragonfly {
		if data.Database.IsNull() {
			rules = append(rules, "$all")
		} else {
			rules = append(rules, fmt.Sprintf("$%d", data.Database.ValueInt64()))
		}
	}

	return rules
}

func parseACLUser(acl []interface{}, data *ACLUserResourceModel, diags *diag.Diagnostics) {
	data.Enabled = types.BoolValue(false)

	// Dragonfly's ACL GETUSER has no "selectors" field, so an untouched list
	// would keep no element type and fail Terraform's type check.
	if data.Selectors.ElementType(context.Background()) == nil {
		data.Selectors = types.ListNull(types.StringType)
	}

	for i := 0; i < len(acl); i += 2 {
		key, ok := acl[i].(string)
		if !ok {
			diags.AddError("Parse Error", "ACL key is not a string")
			return
		}
		v := acl[i+1]

		switch key {
		case "flags":
			flags, ok := v.([]interface{})
			if !ok {
				diags.AddError("Parse Error", "flags not array")
				return
			}
			for _, f := range flags {
				if f.(string) == "on" {
					data.Enabled = types.BoolValue(true)
				}
			}
		case "passwords":
			// Skip passwords
		case "keys":
			var keyStr string
			switch vv := v.(type) {
			case string:
				keyStr = vv
			case []interface{}:
				var parts []string
				for _, p := range vv {
					part, ok := p.(string)
					if !ok {
						diags.AddError("Parse Error", "key part is not a string")
						return
					}
					parts = append(parts, part)
				}
				keyStr = strings.Join(parts, " ")
			default:
				diags.AddError("Parse Error", "keys not string or array")
				return
			}
			data.Keys = types.StringValue(keyStr)
		case "channels":
			var chanStr string
			switch vv := v.(type) {
			case string:
				chanStr = vv
			case []interface{}:
				var parts []string
				for _, p := range vv {
					part, ok := p.(string)
					if !ok {
						diags.AddError("Parse Error", "channel part is not a string")
						return
					}
					parts = append(parts, part)
				}
				chanStr = strings.Join(parts, " ")
			default:
				diags.AddError("Parse Error", "channels not string or array")
				return
			}
			data.Channels = types.StringValue(withoutResetChannels(chanStr))
		case "commands":
			cmdStr, ok := v.(string)
			if !ok {
				diags.AddError("Parse Error", "commands not string")
				return
			}
			data.Commands = types.StringValue(cmdStr)
		case "selectors":
			sels, ok := v.([]interface{})
			if !ok {
				diags.AddError("Parse Error", "selectors not array")
				return
			}
			var selectorStrs []string
			for _, selI := range sels {
				sel, ok := selI.([]interface{})
				if !ok {
					diags.AddError("Parse Error", "selector not array")
					return
				}
				var parts []string
				for j := 0; j < len(sel); j += 2 {
					sk, ok := sel[j].(string)
					if !ok {
						diags.AddError("Parse Error", "selector key is not a string")
						return
					}
					svI := sel[j+1]
					var sv string
					switch svv := svI.(type) {
					case string:
						sv = svv
					case []interface{}:
						var pp []string
						for _, ppp := range svv {
							part, ok := ppp.(string)
							if !ok {
								diags.AddError("Parse Error", "selector value part is not a string")
								return
							}
							pp = append(pp, part)
						}
						sv = strings.Join(pp, " ")
					default:
						diags.AddError("Parse Error", "selector value not string or array")
						return
					}
					if sk == "commands" || sk == "keys" || sk == "channels" {
						parts = append(parts, sv)
					}
				}
				selectorStrs = append(selectorStrs, strings.Join(parts, " "))
			}
			selectors, d := types.ListValueFrom(context.Background(), types.StringType, selectorStrs)
			diags.Append(d...)
			data.Selectors = selectors
		}
	}
}

// withoutResetChannels drops the "resetchannels" token Dragonfly puts in front
// of a user's channel patterns in ACL GETUSER, so it matches the configuration.
func withoutResetChannels(channels string) string {
	var patterns []string
	for _, token := range strings.Fields(channels) {
		if !strings.EqualFold(token, "resetchannels") {
			patterns = append(patterns, token)
		}
	}

	return strings.Join(patterns, " ")
}

// parseWhoAmI returns the user name from an ACL WHOAMI reply. Redis answers
// with the bare name, Dragonfly with "User is <name>".
func parseWhoAmI(reply string) string {
	return strings.TrimPrefix(reply, "User is ")
}

// parseDragonflyDatabase returns the database a user is restricted to, from its
// ACL LIST line ("... $0"), or null when it may use all databases. Dragonfly's
// ACL GETUSER does not report this rule.
func parseDragonflyDatabase(line string) types.Int64 {
	fields := strings.Fields(line)
	for i := len(fields) - 1; i >= 0; i-- {
		if !strings.HasPrefix(fields[i], "$") {
			continue
		}
		if db, err := strconv.ParseInt(fields[i][1:], 10, 64); err == nil {
			return types.Int64Value(db)
		}
		break
	}

	return types.Int64Null()
}

// aclPasswordState returns the password hashes and the nopass flag from an
// ACL GETUSER reply. Dragonfly sends one string of shortened hashes
// ("#b1d8... #9a9a..."), Redis a list of full SHA-256 hashes.
func aclPasswordState(acl []interface{}) ([]string, bool) {
	var hashes []string
	nopass := false

	for i := 0; i+1 < len(acl); i += 2 {
		key, _ := acl[i].(string)
		switch key {
		case "flags":
			flags, _ := acl[i+1].([]interface{})
			for _, flag := range flags {
				if f, ok := flag.(string); ok && strings.EqualFold(f, "nopass") {
					nopass = true
				}
			}
		case "passwords":
			switch v := acl[i+1].(type) {
			case string:
				for _, token := range strings.Fields(v) {
					hashes = append(hashes, strings.TrimPrefix(token, "#"))
				}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						hashes = append(hashes, strings.TrimPrefix(s, "#"))
					}
				}
			}
		}
	}

	return hashes, nopass
}

// passwordMatches reports whether the user has exactly the one password whose
// SHA-256 is sha, and no nopass flag.
func passwordMatches(hashes []string, nopass bool, sha string) bool {
	if nopass || len(hashes) != 1 || len(hashes[0]) < 15 {
		return false
	}

	return strings.HasPrefix(strings.ToLower(sha), strings.ToLower(hashes[0]))
}

// deletionPatternError explains why a delete_keys_on_destroy pattern is not
// allowed, or returns "" when it is.
func deletionPatternError(pattern string) string {
	if pattern == "" {
		return "a key pattern must not be empty"
	}
	if strings.ContainsAny(pattern[:1], "*?[\\") {
		return fmt.Sprintf("key pattern %q must start with a fixed prefix, e.g. \"tenant:*\", so it cannot match every key", pattern)
	}

	return ""
}
