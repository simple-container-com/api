// SPDX-License-Identifier: MIT
// Copyright (c) Simple Container

package evolution

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Artifact Registry statuses, from v1RegistryStatus.
const (
	RegistryStatusCreating = "CREATING"
	RegistryStatusActive   = "ACTIVE"
	RegistryStatusError    = "ERROR"
)

// RegistryTypeDocker is the only registry type that holds container images.
const RegistryTypeDocker = "DOCKER"

// Registry is one Artifact Registry, as the API reports it.
type Registry struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Status       string `json:"status,omitempty"`
	RegistryType string `json:"registryType,omitempty"`
	IsPublic     bool   `json:"isPublic,omitempty"`
	RegistryMode string `json:"registryMode,omitempty"`
}

// Host is the docker host images in this registry are tagged with.
func (r Registry) Host() string { return RegistryHost(r.Name) }

// RegistryHost builds the docker host for a registry name.
func RegistryHost(name string) string {
	return fmt.Sprintf("%s.%s", name, RegistryHostSuffix)
}

type createRegistryRequest struct {
	ProjectID    string `json:"projectId"`
	Name         string `json:"name"`
	RegistryType string `json:"registryType,omitempty"`
	IsPublic     bool   `json:"isPublic,omitempty"`
}

type listRegistriesResponse struct {
	Registries []Registry `json:"registries"`
}

// ListRegistries returns every registry in the project.
func (c *Client) ListRegistries(ctx context.Context) ([]Registry, error) {
	var out listRegistriesResponse
	if err := c.request(ctx, http.MethodGet, c.registryURL, "/v1/registries", c.projectQuery(), nil, &out); err != nil {
		return nil, errors.Wrapf(err, "failed to list cloud.ru artifact registries")
	}
	return out.Registries, nil
}

// FindRegistry returns the registry with the given name, if the project has one.
func (c *Client) FindRegistry(ctx context.Context, name string) (*Registry, bool, error) {
	registries, err := c.ListRegistries(ctx)
	if err != nil {
		return nil, false, err
	}
	for i := range registries {
		if strings.EqualFold(registries[i].Name, name) {
			return &registries[i], true, nil
		}
	}
	return nil, false, nil
}

// EnsureRegistry returns the named DOCKER registry, creating it if the project
// does not have one yet, and waits until it is ACTIVE.
//
// A registry is required, not a convenience: Container Apps resolve images from
// **the calling project's own Artifact Registry and nowhere else**. Docker Hub
// bare, `docker.io/…`, `registry-1.docker.io/…` and even Cloud.ru's own
// quickstart demo image all come back `Failed to resolve image`. So "create a
// registry and push into it" is part of the deploy, not a prerequisite the
// operator is asked to arrange.
//
// Create is a no-op when the registry exists, which is what makes a repeat
// deploy cheap: the registry is looked up by name, and the API has no
// create-if-absent.
func (c *Client) EnsureRegistry(ctx context.Context, name string, timeout time.Duration) (*Registry, error) {
	existing, found, err := c.FindRegistry(ctx, name)
	if err != nil {
		return nil, err
	}
	if found {
		if existing.Status == RegistryStatusError {
			return nil, errors.Errorf("cloud.ru artifact registry %q is in status %s — it cannot serve images and must be fixed or deleted in the console",
				name, RegistryStatusError)
		}
		if existing.Status == RegistryStatusActive {
			return existing, nil
		}
		// Mid-creation from an earlier, interrupted deploy: fall through and wait.
		return c.waitRegistryActive(ctx, name, timeout)
	}

	req := createRegistryRequest{
		ProjectID:    c.creds.ProjectID,
		Name:         name,
		RegistryType: RegistryTypeDocker,
	}
	// projectId rides the body here, not the query — see projectQuery.
	if err := c.request(ctx, http.MethodPost, c.registryURL, "/v1/registries", nil, req, nil); err != nil {
		return nil, errors.Wrapf(err, "failed to create cloud.ru artifact registry %q", name)
	}
	return c.waitRegistryActive(ctx, name, timeout)
}

func (c *Client) waitRegistryActive(ctx context.Context, name string, timeout time.Duration) (*Registry, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		reg, found, err := c.FindRegistry(ctx, name)
		if err != nil {
			return nil, err
		}
		if found {
			last = reg.Status
			switch reg.Status {
			case RegistryStatusActive:
				return reg, nil
			case RegistryStatusError:
				return nil, errors.Errorf("cloud.ru artifact registry %q became %s", name, RegistryStatusError)
			}
		}
		if time.Now().After(deadline) {
			return nil, errors.Errorf("cloud.ru artifact registry %q did not become %s within %s (last status: %q)",
				name, RegistryStatusActive, timeout, last)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
