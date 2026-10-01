// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/redis/go-redis/v9"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &ACLUserResource{}
var _ resource.ResourceWithImportState = &ACLUserResource{}
var _ resource.ResourceWithValidateConfig = &ACLUserResource{}

func NewACLUserResource() resource.Resource {
	return &ACLUserResource{}
}

// ACLUserResource defines the resource implementation.
type ACLUserResource struct {
	redisClient *RedisClient
}

// ACLUserResourceModel describes the resource data model.
type ACLUserResourceModel struct {
	ID                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	Passwords           types.List   `tfsdk:"passwords"`
	PasswordWO          types.String `tfsdk:"password_wo"`
	PasswordWOVersion   types.String `tfsdk:"password_wo_version"`
	Keys                types.String `tfsdk:"keys"`
	Channels            types.String `tfsdk:"channels"`
	Commands            types.String `tfsdk:"commands"`
	Selectors           types.List   `tfsdk:"selectors"`
	Database            types.Int64  `tfsdk:"database"`
	DeleteKeysOnDestroy types.List   `tfsdk:"delete_keys_on_destroy"`
	AllowSelfMutation   types.Bool   `tfsdk:"allow_self_mutation"`
}

func (r *ACLUserResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *ACLUserResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Redis ACL user.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The ID of the user (same as name).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The name of the user.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the user is enabled.",
				Optional:            true,
			},
			"passwords": schema.ListAttribute{
				MarkdownDescription: "A list of passwords for the user.",
				ElementType:         types.StringType,
				Optional:            true,
				Sensitive:           true,
			},
			"password_wo": schema.StringAttribute{
				MarkdownDescription: "Write-only password for the user. Never stored in state; requires Terraform 1.11 or later. Change `password_wo_version` to set a new one.",
				Optional:            true,
				Sensitive:           true,
				WriteOnly:           true,
			},
			"password_wo_version": schema.StringAttribute{
				MarkdownDescription: "Any value; changing it re-applies `password_wo`, e.g. to rotate the password.",
				Optional:            true,
			},
			"keys": schema.StringAttribute{
				MarkdownDescription: "The key patterns the user has access to (space-separated if multiple).",
				Optional:            true,
			},
			"channels": schema.StringAttribute{
				MarkdownDescription: "The channel patterns the user has access to (space-separated if multiple).",
				Optional:            true,
			},
			"commands": schema.StringAttribute{
				MarkdownDescription: "The commands the user can execute (space-separated).",
				Optional:            true,
			},
			"selectors": schema.ListAttribute{
				MarkdownDescription: "A list of selectors for the user (each a string of space-separated rules).",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"database": schema.Int64Attribute{
				MarkdownDescription: "Dragonfly only: restrict the user to this logical database (the `$<n>` rule). Unset allows all databases.",
				Optional:            true,
			},
			"delete_keys_on_destroy": schema.ListAttribute{
				MarkdownDescription: "Key patterns (Redis glob syntax) to delete when the user is destroyed, e.g. the user's own prefixes. Each pattern must start with a fixed prefix. Keys are deleted in `database` (default 0), after the user, so no new keys can be written in between.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"allow_self_mutation": schema.BoolAttribute{
				MarkdownDescription: "Whether to allow the user to modify itself.",
				Optional:            true,
			},
		},
	}
}

func (r *ACLUserResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	redisClient, ok := req.ProviderData.(*RedisClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *RedisClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.redisClient = redisClient
}

func (r *ACLUserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ACLUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	passwordHash, diags := r.setUser(ctx, req.Config, &data, "create")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(storePasswordHash(ctx, resp.Private, passwordHash)...)

	// Set the ID to the user name
	data.ID = data.Name

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLUserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ACLUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	r.redisClient.mutex.Lock()
	defer r.redisClient.mutex.Unlock()

	result, err := r.redisClient.client.Do(ctx, "ACL", "GETUSER", data.Name.ValueString()).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read ACL user result, got error: %s", err))
		return
	}

	var val []interface{}
	switch res := result.(type) {
	case []interface{}:
		val = res
	case map[interface{}]interface{}:
		for k, v := range res {
			val = append(val, k, v)
		}
	default:
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to parse ACL GETUSER response: unexpected type %T", result))
		return
	}

	if len(val) == 0 {
		resp.State.RemoveResource(ctx)
		return
	}

	parseACLUser(val, &data, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	storedHash, diags := req.Private.GetKey(ctx, passwordHashKey)
	resp.Diagnostics.Append(diags...)
	if len(storedHash) > 0 {
		var sha string
		if err := json.Unmarshal(storedHash, &sha); err == nil {
			if hashes, nopass := aclPasswordState(val); !passwordMatches(hashes, nopass, sha) {
				// The password was changed on the server; a different version
				// makes the next plan re-apply password_wo.
				data.PasswordWOVersion = types.StringValue(passwordChangedOutsideTerraform)
			}
		}
	}

	if r.redisClient.dragonfly {
		database, err := r.readDragonflyDatabase(ctx, data.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to read ACL LIST, got error: %s", err))
			return
		}
		data.Database = database
	}

	// If the commands in the state and from the API only differ by the
	// "-@all " prefix, keep the state as is to prevent drift.
	var state ACLUserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stateCommands := state.Commands.ValueString()
	dataCommands := data.Commands.ValueString()

	if stateCommands != dataCommands && dataCommands == "-@all "+stateCommands {
		data.Commands = state.Commands
	}

	// Ensure ID is set
	data.ID = data.Name

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLUserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ACLUserResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check for self-mutation
	if !data.AllowSelfMutation.ValueBool() {
		result, err := r.redisClient.client.Do(ctx, "ACL", "WHOAMI").Result()
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get current user, got error: %s", err))
			return
		}
		reply, ok := result.(string)
		if !ok {
			resp.Diagnostics.AddError("Client Error", "Unable to parse current user response")
			return
		}
		currentUser := parseWhoAmI(reply)
		if currentUser == data.Name.ValueString() {
			resp.Diagnostics.AddError("Self-Mutation Error", "Cannot modify the currently authenticated user without setting allow_self_mutation to true")
			return
		}
	}

	passwordHash, diags := r.setUser(ctx, req.Config, &data, "update")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(storePasswordHash(ctx, resp.Private, passwordHash)...)

	// Ensure ID is set
	data.ID = data.Name

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ACLUserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ACLUserResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check for self-mutation
	if !data.AllowSelfMutation.ValueBool() {
		result, err := r.redisClient.client.Do(ctx, "ACL", "WHOAMI").Result()
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to get current user, got error: %s", err))
			return
		}
		reply, ok := result.(string)
		if !ok {
			resp.Diagnostics.AddError("Client Error", "Unable to parse current user response")
			return
		}
		currentUser := parseWhoAmI(reply)
		if currentUser == data.Name.ValueString() {
			resp.Diagnostics.AddError("Self-Mutation Error", "Cannot delete the currently authenticated user without setting allow_self_mutation to true")
			return
		}
	}

	err := r.redisClient.client.ACLDelUser(ctx, data.Name.ValueString()).Err()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", fmt.Sprintf("Unable to delete ACL user, got error: %s", err))
		return
	}

	// Keys go after the user, so nothing can write new ones in between.
	if !data.DeleteKeysOnDestroy.IsNull() {
		var patterns []string
		resp.Diagnostics.Append(data.DeleteKeysOnDestroy.ElementsAs(ctx, &patterns, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		deleted, err := r.deleteKeys(ctx, data.Database, patterns)
		if err != nil {
			resp.Diagnostics.AddError("Client Error", fmt.Sprintf("User deleted, but deleting its keys failed after %d keys: %s", deleted, err))
			return
		}
		tflog.Info(ctx, "Deleted the user's keys", map[string]interface{}{"user": data.Name.ValueString(), "keys": deleted})
	}

	resp.Diagnostics.Append(r.saveACL(ctx)...)
}

func (r *ACLUserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *ACLUserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data ACLUserResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !data.PasswordWO.IsNull() && !data.Passwords.IsNull() {
		resp.Diagnostics.AddAttributeError(
			path.Root("password_wo"),
			"Conflicting password settings",
			"Set either passwords or password_wo, not both.",
		)
	}

	if !data.DeleteKeysOnDestroy.IsNull() && !data.DeleteKeysOnDestroy.IsUnknown() {
		for i, element := range data.DeleteKeysOnDestroy.Elements() {
			pattern, ok := element.(types.String)
			if !ok || pattern.IsUnknown() {
				continue
			}
			if msg := deletionPatternError(pattern.ValueString()); msg != "" {
				resp.Diagnostics.AddAttributeError(path.Root("delete_keys_on_destroy").AtListIndex(i), "Unsafe Key Pattern", msg)
			}
		}
	}
}

// setUser applies the user's rules with ACL SETUSER and saves the ACL file
// when acl_save is enabled.
func (r *ACLUserResource) setUser(ctx context.Context, config tfsdk.Config, data *ACLUserResourceModel, action string) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	if !r.redisClient.dragonfly && !data.Database.IsNull() {
		diags.AddAttributeError(path.Root("database"), "Unsupported Setting", "database is only supported by Dragonfly.")
		return "", diags
	}

	if r.redisClient.dragonfly && !data.Selectors.IsNull() && len(data.Selectors.Elements()) > 0 {
		diags.AddAttributeError(path.Root("selectors"), "Unsupported Setting", "ACL selectors are not supported by Dragonfly.")
		return "", diags
	}

	var writeOnlyPassword types.String
	diags.Append(config.GetAttribute(ctx, path.Root("password_wo"), &writeOnlyPassword)...)
	if diags.HasError() {
		return "", diags
	}

	rules := buildACLSetUserRules(data, aclRuleOptions{
		dragonfly:         r.redisClient.dragonfly,
		writeOnlyPassword: writeOnlyPassword.ValueString(),
	})

	if err := r.redisClient.client.ACLSetUser(ctx, data.Name.ValueString(), rules...).Err(); err != nil {
		diags.AddError("Client Error", fmt.Sprintf("Unable to %s ACL user, got error: %s", action, err))
		return "", diags
	}

	diags.Append(r.saveACL(ctx)...)

	if writeOnlyPassword.ValueString() == "" {
		return "", diags
	}
	sum := sha256.Sum256([]byte(writeOnlyPassword.ValueString()))

	return hex.EncodeToString(sum[:]), diags
}

func (r *ACLUserResource) saveACL(ctx context.Context) diag.Diagnostics {
	var diags diag.Diagnostics

	if !r.redisClient.aclSave {
		return diags
	}

	if err := r.redisClient.client.Do(ctx, "ACL", "SAVE").Err(); err != nil {
		diags.AddError("Client Error", fmt.Sprintf("ACL SAVE failed, the change is not persisted: %s", err))
	}

	return diags
}

func (r *ACLUserResource) readDragonflyDatabase(ctx context.Context, name string) (types.Int64, error) {
	lines, err := r.redisClient.client.Do(ctx, "ACL", "LIST").StringSlice()
	if err != nil {
		return types.Int64Null(), err
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "user "+strings.ToLower(name)+" ") {
			return parseDragonflyDatabase(line), nil
		}
	}

	return types.Int64Null(), nil
}

const (
	passwordHashKey                 = "password_wo_sha256"
	passwordChangedOutsideTerraform = "changed outside Terraform"
)

type privateStateSetter interface {
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// storePasswordHash keeps the SHA-256 of password_wo in private state, so Read
// can tell when the password on the server no longer matches it.
func storePasswordHash(ctx context.Context, private privateStateSetter, hash string) diag.Diagnostics {
	if hash == "" {
		return private.SetKey(ctx, passwordHashKey, nil)
	}

	return private.SetKey(ctx, passwordHashKey, []byte(strconv.Quote(hash)))
}

// deleteKeys removes every key matching the patterns from the given database
// (0 when unset), in batches, and returns how many it deleted.
func (r *ACLUserResource) deleteKeys(ctx context.Context, database types.Int64, patterns []string) (int64, error) {
	var cmd redis.Cmdable = r.redisClient.client
	if db := database.ValueInt64(); db != 0 {
		single, ok := r.redisClient.client.(*redis.Client)
		if !ok {
			return 0, fmt.Errorf("deleting keys in database %d needs a single-server connection", db)
		}
		conn := single.Conn()
		defer func() { _ = conn.Close() }()
		if err := conn.Select(ctx, int(db)).Err(); err != nil {
			return 0, err
		}
		cmd = conn
	}

	var deleted int64
	for _, pattern := range patterns {
		if msg := deletionPatternError(pattern); msg != "" {
			return deleted, errors.New(msg)
		}
		iter := cmd.Scan(ctx, 0, pattern, 1000).Iterator()
		batch := make([]string, 0, 500)
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			n, err := cmd.Unlink(ctx, batch...).Result()
			deleted += n
			batch = batch[:0]
			return err
		}
		for iter.Next(ctx) {
			batch = append(batch, iter.Val())
			if len(batch) == cap(batch) {
				if err := flush(); err != nil {
					return deleted, err
				}
			}
		}
		if err := iter.Err(); err != nil {
			return deleted, err
		}
		if err := flush(); err != nil {
			return deleted, err
		}
	}

	return deleted, nil
}
