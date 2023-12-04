package api

const PulumiTokenAuthType = "pulumi-token"
const ProvisionerTypePulumi = "pulumi"

// PulumiTokenAuthDescriptor describes the pulumi token auth schema
type PulumiTokenAuthDescriptor struct {
	Value string `json:"value"`
}

type PulumiProvisionerConfig struct {
	/**
	  state-storage:
	    type: gcp-bucket
	    credentials: "${auth:gcloud}"
	    provision: true
	  secrets-provider:
	    type: gcp-kms
	    provision: true
	    credentials: "${auth:gcloud}"

	*/
	StateStorage    PulumiStateStorageConfig    `json:"state-storage"`
	SecretsProvider PulumiSecretsProviderConfig `json:"secrets-provider"`
}

type PulumiStateStorageConfig struct {
	Type        string `json:"type"`
	Credentials string `json:"credentials"`
	Provision   bool   `json:"provision"`
}

type PulumiSecretsProviderConfig struct {
	Type        string `json:"type"`
	Credentials string `json:"credentials"`
	Provision   bool   `json:"provision"`
}

func PulumiReadProvisionerConfig(config any) (any, error) {
	return ConvertDescriptor(config, &PulumiProvisionerConfig{})
}
