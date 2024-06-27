package api

import sdk "github.com/pulumi/pulumi/sdk/v3/go/pulumi"

type RegistrarDescriptor struct {
	Type    string `json:"type" yaml:"type"`
	Config  `json:",inline" yaml:",inline"`
	Inherit `json:",inline" yaml:",inline"`
}

type DnsRecord struct {
	Name     string           `json:"name" yaml:"name"`
	Type     string           `json:"type" yaml:"type"`
	ValueOut sdk.StringOutput `json:"valueOut" yaml:"valueOut"`
	Value    string           `json:"value" yaml:"value"`
	Proxied  bool             `json:"proxied" yaml:"proxied"`
}
