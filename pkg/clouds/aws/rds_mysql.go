package aws

import (
	"github.com/simple-container-com/api/pkg/api"
)

const (
	ResourceTypeRdsMysql = "aws-rds-mysql"
)

type MysqlConfig struct {
	AccountConfig   `json:",inline" yaml:",inline"`
	Name            string  `json:"name" yaml:"name"`
	InstanceClass   string  `json:"instanceClass" yaml:"instanceClass"`
	AllocateStorage *int    `json:"allocateStorage" yaml:"allocateStorage"`
	EngineVersion   string  `json:"engineVersion" yaml:"engineVersion"`
	Username        string  `json:"username" yaml:"username"`
	Password        string  `json:"password" yaml:"password"`
	DatabaseName    *string `json:"databaseName" yaml:"databaseName"`
	EngineName      *string `json:"engineName,omitempty" yaml:"engineName,omitempty"`
	// StorageEncrypted opts into AWS-side encryption-at-rest for the
	// underlying EBS volume. When unset (nil), the instance is created
	// with the AWS default — currently UNENCRYPTED — preserving exact
	// behaviour for stacks that pre-date this field. Set `true` to opt
	// the resource into encryption (uses the AWS-managed `aws/rds` KMS
	// key by default).
	//
	// AWS RDS `storage_encrypted` is IMMUTABLE post-creation. Toggling
	// this field on an existing unencrypted instance does NOT migrate
	// data — it is silenced via `pulumi.IgnoreChanges` to prevent a
	// destructive replacement. To convert an existing unencrypted RDS
	// to encrypted, snapshot → encrypted-copy → restore → re-import.
	StorageEncrypted *bool `json:"storageEncrypted,omitempty" yaml:"storageEncrypted,omitempty"`
}

func ReadRdsMysqlConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &MysqlConfig{})
}
