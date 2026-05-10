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
	// StorageEncrypted controls AWS-side encryption-at-rest for the
	// underlying EBS volume. When unset (nil), new instances default
	// to ENCRYPTED (AWS-managed `aws/rds` KMS key), matching CIS-AWS
	// Foundations RDS.3. Set `false` explicitly to opt out for legacy
	// unencrypted stacks; set `true` to be explicit.
	//
	// AWS RDS `storage_encrypted` is IMMUTABLE post-creation. The
	// default flip is safe for existing instances because the
	// `pulumi.IgnoreChanges` on the resource opts (see
	// pkg/clouds/pulumi/aws/rds_mysql.go) silences storage_encrypted
	// drift — Pulumi will not propose a destructive replacement when
	// the spec value differs from the cloud-actual value. Customers
	// who want to genuinely migrate an existing unencrypted RDS to
	// encrypted must do it out-of-band: snapshot → encrypted-copy →
	// restore → re-import.
	StorageEncrypted *bool `json:"storageEncrypted,omitempty" yaml:"storageEncrypted,omitempty"`
}

func ReadRdsMysqlConfig(config *api.Config) (api.Config, error) {
	return api.ConvertConfig(config, &MysqlConfig{})
}
