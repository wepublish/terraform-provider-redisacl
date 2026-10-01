// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	dragonflyImage    = "docker.dragonflydb.io/dragonflydb/dragonfly:v2.0.0"
	dragonflyPassword = "testpass"
	dragonflyACLFile  = "/data/users.acl"
)

var (
	dragonflyContainer testcontainers.Container
	dragonflyAddr      string
)

// StartDragonflyContainer starts a Dragonfly container with an aclfile, the
// setup the provider's acl_save option is meant for.
func StartDragonflyContainer(ctx context.Context) error {
	if dragonflyContainer != nil {
		return nil
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        dragonflyImage,
			ExposedPorts: []string{"6379/tcp"},
			// Dragonfly reserves memory per thread and refuses to start on
			// machines with many cores but little RAM unless both are capped.
			Cmd: []string{
				"--requirepass=" + dragonflyPassword,
				"--aclfile=" + dragonflyACLFile,
				"--proactor_threads=2",
				"--maxmemory=1gb",
			},
			WaitingFor: wait.ForListeningPort("6379/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		return fmt.Errorf("failed to start Dragonfly container: %w", err)
	}
	dragonflyContainer = container

	host, err := container.Host(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Dragonfly host: %w", err)
	}
	port, err := container.MappedPort(ctx, "6379")
	if err != nil {
		return fmt.Errorf("failed to get Dragonfly port: %w", err)
	}
	dragonflyAddr = fmt.Sprintf("%s:%s", host, port.Port())

	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()
	for i := 0; i < 30; i++ {
		if err = admin.Ping(ctx).Err(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}

	return fmt.Errorf("dragonfly not ready: %w", err)
}

func StopDragonflyContainer(ctx context.Context) error {
	if dragonflyContainer == nil {
		return nil
	}
	err := dragonflyContainer.Terminate(ctx)
	dragonflyContainer = nil

	return err
}

func dragonflyAdminClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: dragonflyAddr, Password: dragonflyPassword})
}

func dragonflyUserClient(username, password string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: dragonflyAddr, Username: username, Password: password})
}

// dragonflySavedACL returns the aclfile as written by ACL SAVE.
func dragonflySavedACL(ctx context.Context) (string, error) {
	code, reader, err := dragonflyContainer.Exec(ctx, []string{"cat", dragonflyACLFile})
	if err != nil {
		return "", err
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if code != 0 {
		if strings.Contains(string(output), "No such file or directory") {
			return "", nil
		}
		return "", fmt.Errorf("cat %s exited with %d: %s", dragonflyACLFile, code, output)
	}

	return string(output), nil
}

func cleanupDragonflyUsers(ctx context.Context) error {
	admin := dragonflyAdminClient()
	defer func() { _ = admin.Close() }()

	users, err := admin.Do(ctx, "ACL", "USERS").StringSlice()
	if err != nil {
		return err
	}
	for _, user := range users {
		if user != "default" {
			if err := admin.ACLDelUser(ctx, user).Err(); err != nil {
				return err
			}
		}
	}

	return nil
}
