package api

import (
	"context"

	"github.com/simple-container-com/api/pkg/api/logger"
)

// noopProvisioner is a minimal api.Provisioner implementation used to exercise
// provisioner-registration and descriptor-detection code paths without a real
// cloud backend.
type noopProvisioner struct {
	pubKey       string
	configReader ProvisionerFieldConfigReaderFunc
}

func (n *noopProvisioner) ProvisionStack(context.Context, *ConfigFile, Stack, ProvisionParams) error {
	return nil
}
func (n *noopProvisioner) SetPublicKey(pubKey string) { n.pubKey = pubKey }
func (n *noopProvisioner) DeployStack(context.Context, *ConfigFile, Stack, DeployParams) error {
	return nil
}
func (n *noopProvisioner) DestroyChildStack(context.Context, *ConfigFile, Stack, DestroyParams, bool) error {
	return nil
}
func (n *noopProvisioner) PreviewStack(context.Context, *ConfigFile, Stack, ProvisionParams) (*PreviewResult, error) {
	return &PreviewResult{}, nil
}
func (n *noopProvisioner) PreviewChildStack(context.Context, *ConfigFile, Stack, DeployParams) (*PreviewResult, error) {
	return &PreviewResult{}, nil
}
func (n *noopProvisioner) OutputsStack(context.Context, *ConfigFile, Stack, StackParams) (*OutputsResult, error) {
	return &OutputsResult{}, nil
}
func (n *noopProvisioner) CancelStack(context.Context, *ConfigFile, Stack, StackParams) error {
	return nil
}
func (n *noopProvisioner) DestroyParentStack(context.Context, *ConfigFile, Stack, DestroyParams, bool) error {
	return nil
}
func (n *noopProvisioner) SetConfigReader(f ProvisionerFieldConfigReaderFunc) { n.configReader = f }

// fakeAuth implements the AuthConfig interface; CredentialsValue returns a raw
// string so ConvertAuth can JSON-decode it.
type fakeAuth struct {
	cred      string
	projectID string
}

func (f *fakeAuth) ProviderType() string     { return "fake" }
func (f *fakeAuth) CredentialsValue() string  { return f.cred }
func (f *fakeAuth) ProjectIdValue() string    { return f.projectID }

// fakeCloudHelper implements api.CloudHelper for NewCloudHelper tests.
type fakeCloudHelper struct{ logger any }

func (f *fakeCloudHelper) Run() error          { return nil }
func (f *fakeCloudHelper) SetLogger(l logger.Logger) { f.logger = l }
