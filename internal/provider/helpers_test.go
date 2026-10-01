// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/stretchr/testify/assert"
)

func TestParseACLUser(t *testing.T) {
	tests := []struct {
		name     string
		acl      []interface{}
		expected *ACLUserResourceModel
	}{
		{
			name: "simple user",
			acl: []interface{}{
				"flags", []interface{}{"on"},
				"passwords", []interface{}{},
				"keys", "~*",
				"channels", "&*",
				"commands", "+@all",
			},
			expected: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Keys:     types.StringValue("~*"),
				Channels: types.StringValue("&*"),
				Commands: types.StringValue("+@all"),
			},
		},
		{
			name: "disabled user",
			acl: []interface{}{
				"flags", []interface{}{"off"},
				"passwords", []interface{}{},
				"keys", "~somekey",
				"channels", "&somechannel",
				"commands", "-@all",
			},
			expected: &ACLUserResourceModel{
				Enabled:  types.BoolValue(false),
				Keys:     types.StringValue("~somekey"),
				Channels: types.StringValue("&somechannel"),
				Commands: types.StringValue("-@all"),
			},
		},
		{
			name: "user with selectors",
			acl: []interface{}{
				"flags", []interface{}{"on"},
				"passwords", []interface{}{},
				"keys", "~*",
				"channels", "&*",
				"commands", "+@all",
				"selectors", []interface{}{
					[]interface{}{"commands", "somecommand"},
				},
			},
			expected: &ACLUserResourceModel{
				Enabled:   types.BoolValue(true),
				Keys:      types.StringValue("~*"),
				Channels:  types.StringValue("&*"),
				Commands:  types.StringValue("+@all"),
				Selectors: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("somecommand")}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			actual := &ACLUserResourceModel{}
			parseACLUser(tt.acl, actual, &diags)

			assert.Empty(t, diags)
			assert.Equal(t, tt.expected.Enabled, actual.Enabled)
			assert.Equal(t, tt.expected.Keys, actual.Keys)
			assert.Equal(t, tt.expected.Channels, actual.Channels)
			assert.Equal(t, tt.expected.Commands, actual.Commands)
			if tt.expected.Selectors.IsNull() {
				assert.True(t, actual.Selectors.IsNull())
			} else {
				assert.True(t, tt.expected.Selectors.Equal(actual.Selectors))
			}
		})
	}
}

func TestBuildACLSetUserRules(t *testing.T) {
	tests := []struct {
		name     string
		data     *ACLUserResourceModel
		expected []string
	}{
		{
			name: "basic enabled user",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Keys:     types.StringValue("~*"),
				Channels: types.StringValue("&*"),
				Commands: types.StringValue("+@all"),
			},
			expected: []string{"reset", "on", "resetkeys", "~*", "resetchannels", "&*", "-@all", "+@all"},
		},
		{
			name: "disabled user",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(false),
				Keys:     types.StringValue("~key*"),
				Channels: types.StringValue("&channel*"),
				Commands: types.StringValue("-@all +get"),
			},
			expected: []string{"reset", "off", "resetkeys", "~key*", "resetchannels", "&channel*", "-@all", "+get"},
		},
		{
			name: "commands without -@all prefix should get it added",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Commands: types.StringValue("+get +set"),
			},
			expected: []string{"reset", "on", "~*", "&*", "-@all", "+get", "+set"},
		},
		{
			name: "commands with -@all prefix should not get duplicate",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Commands: types.StringValue("-@all +get +set"),
			},
			expected: []string{"reset", "on", "~*", "&*", "-@all", "+get", "+set"},
		},
		{
			name: "user with single password",
			data: &ACLUserResourceModel{
				Enabled:   types.BoolValue(true),
				Passwords: types.ListValueMust(types.StringType, []attr.Value{types.StringValue("password123")}),
			},
			expected: []string{"reset", "on", "resetpass", ">password123", "~*", "&*", "+@all"},
		},
		{
			name: "user with multiple passwords",
			data: &ACLUserResourceModel{
				Enabled: types.BoolValue(true),
				Passwords: types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue("password1"),
					types.StringValue("password2"),
					types.StringValue("password3"),
				}),
			},
			expected: []string{"reset", "on", "resetpass", ">password1", ">password2", ">password3", "~*", "&*", "+@all"},
		},
		{
			name: "nopass user (empty password list)",
			data: &ACLUserResourceModel{
				Enabled:   types.BoolValue(true),
				Passwords: types.ListValueMust(types.StringType, []attr.Value{}),
			},
			expected: []string{"reset", "on", "resetpass", "nopass", "~*", "&*", "+@all"},
		},
		{
			name: "user with multiple key patterns",
			data: &ACLUserResourceModel{
				Enabled: types.BoolValue(true),
				Keys:    types.StringValue("~key1* ~key2* ~key3*"),
			},
			expected: []string{"reset", "on", "resetkeys", "~key1*", "~key2*", "~key3*", "&*", "+@all"},
		},
		{
			name: "user with multiple channel patterns",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Channels: types.StringValue("&channel1* &channel2*"),
			},
			expected: []string{"reset", "on", "~*", "resetchannels", "&channel1*", "&channel2*", "+@all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := buildACLSetUserRules(tt.data, aclRuleOptions{})
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestBuildACLSetUserRules_Comprehensive(t *testing.T) {
	tests := []struct {
		name     string
		data     *ACLUserResourceModel
		expected []string
	}{
		{
			name: "all parameters null",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolNull(),
				Keys:     types.StringNull(),
				Channels: types.StringNull(),
				Commands: types.StringNull(),
			},
			expected: []string{"reset", "on", "~*", "&*", "+@all"},
		},
		{
			name: "with selectors",
			data: &ACLUserResourceModel{
				Enabled: types.BoolValue(true),
				Selectors: types.ListValueMust(types.StringType, []attr.Value{
					types.StringValue("~key1* +get"),
					types.StringValue("~key2* +set"),
				}),
			},
			expected: []string{"reset", "on", "~*", "&*", "+@all", "(~key1* +get)", "(~key2* +set)"},
		},
		{
			name: "complex command combinations",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Commands: types.StringValue("-@all +@read +@write -del"),
			},
			expected: []string{"reset", "on", "~*", "&*", "-@all", "+@read", "+@write", "-del"},
		},
		{
			name: "empty strings for keys and channels",
			data: &ACLUserResourceModel{
				Enabled:  types.BoolValue(true),
				Keys:     types.StringValue(""),
				Channels: types.StringValue(""),
				Commands: types.StringValue(""),
			},
			expected: []string{"reset", "on", "resetkeys", "resetchannels", "-@all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := buildACLSetUserRules(tt.data, aclRuleOptions{})
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestBuildACLSetUserRules_Dragonfly(t *testing.T) {
	data := &ACLUserResourceModel{
		Enabled:  types.BoolValue(true),
		Keys:     types.StringValue("~wepublish-demo-staging:* ~{wepublish-demo-staging}:*"),
		Channels: types.StringValue("&wepublish-demo-staging:*"),
		Commands: types.StringValue("+@all -@dangerous +info"),
	}

	actual := buildACLSetUserRules(data, aclRuleOptions{dragonfly: true, writeOnlyPassword: "secret"})

	assert.NotContains(t, actual, "reset", "Dragonfly rejects the reset rule")
	assert.Equal(t, []string{
		"resetpass", "resetkeys", "resetchannels", "-@all",
		"on",
		"resetpass", ">secret",
		"resetkeys", "~wepublish-demo-staging:*", "~{wepublish-demo-staging}:*",
		"resetchannels", "&wepublish-demo-staging:*",
		"-@all", "+@all", "-@dangerous", "+info",
		"$all",
	}, actual)
}

func TestBuildACLSetUserRules_DragonflyDatabase(t *testing.T) {
	data := &ACLUserResourceModel{Enabled: types.BoolValue(true), Database: types.Int64Value(0)}

	actual := buildACLSetUserRules(data, aclRuleOptions{dragonfly: true})

	assert.Equal(t, "$0", actual[len(actual)-1])
	assert.NotContains(t, actual, "$all")
}

func TestBuildACLSetUserRules_RedisNeverSendsDatabase(t *testing.T) {
	data := &ACLUserResourceModel{Enabled: types.BoolValue(true)}

	for _, rule := range buildACLSetUserRules(data, aclRuleOptions{}) {
		assert.NotRegexp(t, `^\$`, rule)
	}
}

func TestParseDragonflyDatabase(t *testing.T) {
	tests := []struct {
		line     string
		expected types.Int64
	}{
		{"user u on #abc ~u:* resetchannels &u:* +@all -@dangerous +info $0", types.Int64Value(0)},
		{"user u on #abc ~u:* &* +@all $7", types.Int64Value(7)},
		{"user u on #abc ~u:* &* +@all $all", types.Int64Null()},
		{"user u on #abc ~* &* +@all", types.Int64Null()},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, parseDragonflyDatabase(tt.line), tt.line)
	}
}

func TestBuildACLSetUserRules_WriteOnlyPassword(t *testing.T) {
	data := &ACLUserResourceModel{Enabled: types.BoolValue(true)}

	actual := buildACLSetUserRules(data, aclRuleOptions{writeOnlyPassword: "secret"})

	assert.Equal(t, []string{"reset", "on", "resetpass", ">secret", "~*", "&*", "+@all"}, actual)
}

func TestParseACLUser_DragonflyChannels(t *testing.T) {
	acl := []interface{}{
		"flags", []interface{}{"on"},
		"passwords", "#30c952fab122c3f",
		"commands", "+@all -@dangerous +info",
		"keys", "~u1:* ~{u1}:*",
		"channels", "resetchannels &u1:*",
	}
	actual := &ACLUserResourceModel{}
	var diags diag.Diagnostics

	parseACLUser(acl, actual, &diags)

	assert.False(t, diags.HasError())
	assert.Equal(t, types.StringValue("&u1:*"), actual.Channels)
	assert.Equal(t, types.StringValue("~u1:* ~{u1}:*"), actual.Keys)
	assert.Equal(t, types.StringValue("+@all -@dangerous +info"), actual.Commands)
}

func TestParseWhoAmI(t *testing.T) {
	assert.Equal(t, "default", parseWhoAmI("default"), "Redis")
	assert.Equal(t, "default", parseWhoAmI("User is default"), "Dragonfly")
}

func TestParseACLUser_WithoutSelectorsField(t *testing.T) {
	acl := []interface{}{
		"flags", []interface{}{"on"},
		"commands", "+@all",
		"keys", "~*",
		"channels", "&*",
	}
	actual := &ACLUserResourceModel{}
	var diags diag.Diagnostics

	parseACLUser(acl, actual, &diags)

	assert.False(t, diags.HasError())
	assert.Equal(t, types.ListNull(types.StringType), actual.Selectors)
}

func TestACLPasswordState(t *testing.T) {
	tests := []struct {
		name   string
		acl    []interface{}
		hashes []string
		nopass bool
	}{
		{
			name:   "Dragonfly, two passwords",
			acl:    []interface{}{"flags", []interface{}{"on"}, "passwords", "#b1d82041a9721aa #9a9a7e41e40519f"},
			hashes: []string{"b1d82041a9721aa", "9a9a7e41e40519f"},
		},
		{
			name:   "Redis, full hashes",
			acl:    []interface{}{"flags", []interface{}{"on"}, "passwords", []interface{}{"b1d82041a9721aaaeaf2b021df203cb1138a037c050da0f16351b17c2f48e952"}},
			hashes: []string{"b1d82041a9721aaaeaf2b021df203cb1138a037c050da0f16351b17c2f48e952"},
		},
		{
			name:   "nopass",
			acl:    []interface{}{"flags", []interface{}{"on", "nopass"}, "passwords", ""},
			nopass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashes, nopass := aclPasswordState(tt.acl)
			assert.Equal(t, tt.hashes, hashes)
			assert.Equal(t, tt.nopass, nopass)
		})
	}
}

func TestPasswordMatches(t *testing.T) {
	sha := "b1d82041a9721aaaeaf2b021df203cb1138a037c050da0f16351b17c2f48e952"

	assert.True(t, passwordMatches([]string{"b1d82041a9721aa"}, false, sha), "Dragonfly shortened hash")
	assert.True(t, passwordMatches([]string{sha}, false, sha), "Redis full hash")
	assert.False(t, passwordMatches([]string{"b1d82041a9721aa", "9a9a7e41e40519f"}, false, sha), "an extra password")
	assert.False(t, passwordMatches([]string{"9a9a7e41e40519f"}, false, sha), "a different password")
	assert.False(t, passwordMatches(nil, true, sha), "nopass")
	assert.False(t, passwordMatches([]string{""}, false, sha), "empty hash")
}

func TestDeletionPatternError(t *testing.T) {
	for _, pattern := range []string{"wepublish-bajour-production:*", "{wepublish-bajour-production}:*", "cache"} {
		assert.Empty(t, deletionPatternError(pattern), pattern)
	}
	for _, pattern := range []string{"", "*", "?x:*", "[ab]:*", "\\*:*"} {
		assert.NotEmpty(t, deletionPatternError(pattern), pattern)
	}
}
