package api

const CloudflareRegistrarType = "cloudflare"

type CloudflareRegisrarConfig struct {
	Credentials string                `json:"credentials"`
	Project     string                `json:"project"`
	ZoneName    string                `json:"zoneName"`
	DNSRecords  []CloudflareDNSRecord `json:"dnsRecords"`
}

type CloudflareDNSRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}
