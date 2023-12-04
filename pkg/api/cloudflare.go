package api

const RegistrarTypeCloudflare = "cloudflare"

type CloudflareRegistrarConfig struct {
	/**
	  credentials: "${secret:CLOUDFLARE_API_TOKEN}"
	  project: sc-refapp
	  zoneName: sc-refapp.org
	  dnsRecords:
	    - name: "@"
	      type: "TXT"
	      value: "MS=ms83691649"
	*/
	Credentials string                `json:"credentials"`
	Project     string                `json:"project"`
	ZoneName    string                `json:"zoneName"`
	DnsRecords  []CloudflareDnsRecord `json:"dnsRecords"`
}

type CloudflareDnsRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

func CloudflareReadRegistrarConfig(config any) (any, error) {
	return ConvertDescriptor(config, &CloudflareRegistrarConfig{})
}
