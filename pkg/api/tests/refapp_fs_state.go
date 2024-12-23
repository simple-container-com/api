package tests

import (
	"fmt"
	"os/user"

	"github.com/simple-container-com/api/pkg/api"
	"github.com/simple-container-com/api/pkg/clouds/fs"
	"github.com/simple-container-com/api/pkg/clouds/pulumi"
)

func ResolvedFsStateStorage() *fs.FileSystemStateStorage {
	var usr *user.User
	usr, _ = user.Current()

	return &fs.FileSystemStateStorage{
		Path: fmt.Sprintf("file:///%s/.sc/pulumi/state", usr.HomeDir),
	}
}

func ResolvedFsStateStorageServerDescriptor() api.ServerDescriptor {
	desc := ResolvedCommonServerDescriptor.Copy()
	desc.Provisioner = api.ProvisionerDescriptor{
		Type: pulumi.ProvisionerTypePulumi,
		Config: api.Config{Config: &pulumi.ProvisionerConfig{
			StateStorage: pulumi.StateStorageConfig{
				Type:   pulumi.StateStorageTypeFileSystem,
				Config: api.Config{Config: ResolvedFsStateStorage()},
			},
			SecretsProvider: pulumi.SecretsProviderConfig{
				Type: pulumi.SecretsProviderTypePassPhrase,
				Config: api.Config{Config: &fs.PassphraseSecretsProvider{
					PassPhrase: "pass-phrase",
				}},
			},
		}},
	}
	return desc
}
