# DNS Management with Simple Container

This comprehensive guide covers how to configure and manage DNS records and domain routing using Simple Container's integrated DNS management capabilities. Simple Container provides seamless domain management through Cloudflare integration with full DNS record control and security features.

## Overview

Simple Container's DNS management provides:

- **Cloudflare Integration** for reliable DNS hosting and CDN capabilities
- **Declarative Configuration** for version-controlled domain management
- **Multi-Environment Support** with environment-specific domains
- **Security Features** including Cloudflare-only ingress protection
- **Email Infrastructure** with SPF, DKIM, and DMARC record support
- **Template Placeholders** for dynamic domain generation
- **Load Balancer Integration** with automatic DNS record creation

## DNS Architecture

Simple Container uses a resource-based approach to DNS management:

1. **Parent Stack** defines DNS resources and capabilities
2. **Child Applications** reference domains in their client.yaml configuration
3. **Environment-Specific** domains are automatically managed
4. **Security Policies** are enforced at the DNS level

## Prerequisites

Before setting up DNS management, ensure you have:

- Cloudflare account with API access
- Domain registered and using Cloudflare nameservers
- Simple Container CLI installed
- Parent stack with DNS resource configured

## Setting Up DNS Resources

### 1. Parent Stack Configuration

Define DNS management resources in your parent stack's `server.yaml`:

```yaml
# server.yaml - Parent Stack
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        - name: "api"
          type: "CNAME"
          value: "api-lb.aws.mycompany.com"
        - name: "www"
          type: "CNAME"
          value: "mycompany.com"
        - name: "@"
          type: "A"
          value: "203.0.113.10"
        - name: "@"
          type: "TXT"
          value: "v=spf1 include:_spf.google.com ~all"
```

### 2. Authentication Setup

Configure Cloudflare credentials in your `secrets.yaml`:

```yaml
# secrets.yaml
auth:
  cloudflare:
    credentials: "${secret:cloudflare-api-token}"

values:
  cloudflare-api-token: "your-cloudflare-api-token"
  cloudflare-account-id: "your-cloudflare-account-id"
```

### 3. Yandex Cloud DNS Registrar (`yc-dns`)

For `.ru` and other zones hosted on Yandex Cloud DNS, use the `yc-dns` registrar.
Two things are different from the Cloudflare shape:

- **The edge is an API Gateway, not a CDN.** The Cloudflare registrar hangs a
  proxied CNAME plus a `Host`-rewriting Worker in front of your service. The
  `yc-dns` registrar instead creates a **YC API Gateway** resource that
  terminates TLS from the adopted certificate and forwards the target's own
  `Host` to the container URL. This is what makes TLS + a real edge work on a
  `.ru` service domain without paying the CDN's per-resource flat fee — and
  it dodges the wildcard-CDN limitation (one CDN resource per container is
  required, since the container hostname *is* the identity, and YC CDN keys
  the cache on URI alone so byte-identical responses on distinct hostnames
  leak across services).

- **Certificates are adopted, never issued.** `certificateId` is required on
  every `yc-dns` registrar declaration and must reference a wildcard cert that
  covers the zone. A managed DNS_CNAME certificate validates through the
  single `_acme-challenge.<zone>` CNAME target, so a second managed cert for
  the same domain contends with the first's renewal and fails as an expiry up
  to 90 days later. Adopt one wildcard cert per zone and share it across
  services.

```yaml
# server.yaml — declaring multiple registrars side-by-side is supported
# via the `registrars:` (plural) map. SC routes each domain to the entry
# whose zone is the longest declared suffix, on a label boundary.
resources:
  registrars:
    cloudflare:
      type: cloudflare
      config:
        credentials: "${auth:cloudflare}"
        accountId: "${secret:cloudflare-account-id}"
        zoneName: "mycompany.com"
    yandex:
      type: yc-dns
      config:
        credentials: "${auth:yc}"
        # The YC *resource* name of the DNS zone, not the DNS name of it.
        # `example.ru.` is served by a resource named `example-ru`. Set
        # `zoneId:` directly to skip the lookup.
        zoneName: example-ru
        # Adopted wildcard certificate — one per zone.
        certificateId: fpqu...
```

**Refused records.** `NS`, `SOA` and anything under `_acme-challenge.` are
refused at conversion time — overwriting NS takes the zone off the air,
overwriting `_acme-challenge.` breaks renewal up to 90 days later.

**TTL.** Defaults to 300 seconds. YC does not treat `1` as the "automatic"
sentinel that some other providers use.

**Multi-valued records.** `Datas` is a list, so one recordset can hold N
values; unlike Cloudflare where a multi-valued record is N separate resources.

**`Proxied` is ignored.** The Cloudflare-only `proxied` flag has no meaning on
`yc-dns` and is dropped.

For the full YC parent-stack shape and destroy hazards, see
[Yandex Cloud](parent-yandex-cloud.md).

### 4. Cloud.ru Cloud DNS Registrar (`cloudru-dns`)

For zones hosted on **Cloud.ru Evolution**'s Cloud DNS. Same role as `yc-dns`: SC
writes the records a service's `domain:` needs into a zone that already exists.

> **Implemented but not yet smoked end to end.** The routes and field names are a
> faithful transcription of Cloud.ru's published API reference, not measurements
> against a live project — the service-account key this driver was built with lacks
> the Cloud DNS role, and every call answers
> `403 {"code":7,"message":"permission denied: invalid scopes"}`. Expect the first
> live use to correct something, and read `pkg/clouds/cloudru/evolution/dns.go`'s
> header for the three most likely candidates. Everything else on Cloud.ru —
> Container Apps, both VM drivers, k3s — is live-verified; this one is not.

```yaml
# server.yaml
resources:
  registrars:
    cloudru:
      type: cloudru-dns
      config:
        # The Evolution service-account key — the same credential the Container App
        # and VM drivers use. Written VERBATIM: `${secret:...}` is not expanded
        # inside an auth block, and SC refuses an unexpanded placeholder rather
        # than letting IAM reject it mid-deploy.
        credentials: "${auth:cloudru}"
        # The zone as it appears in a domain name.
        zoneName: simple-forge.ru
        # PREFER this: the zone's own id, which is the path segment of its console
        # URL — /spa/dns/zones/public/<zoneId>/edit. Needs nothing else.
        zoneId: "11111111-2222-3333-4444-555555555555"
        # Alternative to zoneId: scope a lookup-by-name to a Cloud DNS parent.
        # Only one of the two is required — see the warning below before reaching
        # for this one.
        # parentId: "..."
        # Optional, default 3600. Cloud DNS has no automatic-TTL sentinel.
        # recordTtl: 300
```

> **Use `zoneId`, not `parentId`.** Open the zone in the console and the id is right
> there in the URL path: `/spa/dns/zones/public/<zoneId>/edit`. It identifies the zone
> outright, so no lookup — and no parent scope — is needed.
>
> **`parentId` is the one value here we cannot tell you how to find.** It scopes a
> lookup *by name*, and a wrong value **does not fail**: Cloud DNS returns an *empty
> set of zones*, which is indistinguishable from a parent that has no zones. An
> earlier version of this page said it was a "Cloud DNS service instance id, visible
> at `console.cloud.ru/spa/cloud-dns/<parentId>`" — that URL shape does not exist. A
> real zone URL exposes a zone id, a `customerId` and a `projectId`, and no fourth
> id; which of those `parentId` wants has never been established, because every Cloud
> DNS call from a service account without the DNS role answers `403` regardless of
> the value. So reach for `zoneId` and sidestep the question.
> `sc cloudru dns-record apply` prints the zones it can see when a lookup comes up
> empty.

**The zone is looked up, never created** — the same rule as `yc-dns`, and for the same
reason: a public zone is authoritative for a real domain, so creating one from a deploy
either collides with the delegation that exists or produces a second zone nobody's
nameservers point at.

#### Getting the zone ready: delegation, and maybe verification

**Point the domain's nameservers at the pair your zone publishes.** Open the zone in the
console and read the **NS-серверы** field — for an Evolution zone that is:

```
evo-cns01.cloud.ru
evo-cns02.cloud.ru
```

**Read it off your own zone rather than copying the pair above**, because the two
Cloud.ru estates do not share nameservers and the documentation mostly does not say
which estate it is describing. Cloud.ru's Cloud DNS quickstart and delegation guide both
name `cdns-ns01/cdns-ns02.sbercloud.ru` — that is the **Advanced** (ex-SberCloud) estate.
An Evolution zone publishes the `evo-` pair above, which is a genuinely different
network (`94.158.23.11` / `94.158.23.22`, against `37.18.8.44` / `37.18.8.88`). SC keeps
the Evolution pair in `evolution.DnsNameservers` as a fallback and prefers your zone's
own list when the API returns one.

Delegation is **not enforced**: Cloud DNS reports `delegated: false` and accepts the
write anyway, so a record in an undelegated zone is **created successfully and resolves
nowhere**. SC warns on it and continues — but only when the API affirmatively says
`false`, so a silent API does not produce a warning on every deploy. If a new record
never appears in `dig`, check the delegation before you check SC.

**Zone verification (`valid`) may not apply on Evolution.** The API reference documents
`valid` / `validationText` — a TXT record at the apex carrying a value shaped
`sbc-verification: <uuid>`, proving you control the domain, with Cloud DNS refusing
record creation until it clears. An Evolution zone created in the console on 2026-10-24
went straight to **Активен** with its NS pair published and **no verification value
anywhere on the page**, which is the DNS-hosting model rather than the
domain-verification one. So this is likely another Advanced-estate concept in shared
documentation.

SC handles both worlds by acting only on what the API actually says: it refuses the
deploy — quoting the value to publish — when a zone comes back `valid: false`, and
proceeds normally when the field is simply absent. **If your zone page does show a
verification value, publish it before you move the nameservers.** Verification needs a
TXT record served by whoever is authoritative *now*; switch NS first and you get a window
where Cloud.ru is authoritative for the domain, still refuses to hold records for it, and
cannot be verified — the domain goes dark and the SC deploy is what fails.

**Multi-valued records** are one recordset with N values, as on `yc-dns` and unlike
Cloudflare. **`proxied`** has no meaning here and is dropped.

A standalone CLI exists for working on records without a deploy:

```bash
sc cloudru dns-record apply    # create/update
sc cloudru dns-record destroy
```

### Which registrars exist today

| Registrar | Zone host | Status |
|---|---|---|
| `cloudflare` | Cloudflare | live, the default on `.com` zones |
| `yc-dns` | Yandex Cloud DNS | live |
| `cloudru-dns` | Cloud.ru Evolution Cloud DNS | implemented, **not smoked** (above) |

A cloud with no registrar entry here does not mean no DNS — it means you create the
record yourself. On a `vm`/k3s stand that is an A record at the machine's address,
**not proxied** (the node answers the ACME challenge itself, so Cloudflare's orange
cloud breaks it, which is why `domainProxied` is refused on those paths rather than
accepted and ignored).

## Application Domain Configuration

### 1. Basic Domain Assignment

Configure domains in your application's `client.yaml`:

```yaml
# client.yaml
stacks:
  staging:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: staging-api.mycompany.com
      dockerComposeFile: docker-compose.yaml
      runs: [web-app]
      
  production:
    type: cloud-compose
    parent: myorg/infrastructure
    config:
      domain: api.mycompany.com
      dockerComposeFile: docker-compose.yaml
      runs: [web-app]
```

### 2. Dynamic Domain Generation

Use template placeholders for automatic domain generation:

```yaml
# Environment-based domains
stacks:
  staging:
    config:
      domain: "${stack:name}-api.mycompany.com"  # staging-api.mycompany.com
      
  production:
    config:
      domain: "api.mycompany.com"  # Production root domain
```

### 3. Environment Variable Domains

Support runtime domain configuration:

```yaml
# client.yaml
stacks:
  staging:
    config:
      domain: "${env:STAGING_DOMAIN:staging.mycompany.com}"
      env:
        BASE_URL: "https://${env:STAGING_DOMAIN:staging.mycompany.com}"
```

## Advanced DNS Features

### 1. Email Infrastructure

Configure email-related DNS records:

```yaml
# server.yaml - Email infrastructure
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        # SPF Record
        - name: "@"
          type: "TXT"
          value: "v=spf1 include:_spf.google.com include:_spf.hubspot.com ~all"
        
        # DKIM Record for Google Workspace
        - name: "google._domainkey"
          type: "TXT"
          value: "v=DKIM1; k=rsa; p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC..."
        
        # DMARC Policy
        - name: "_dmarc"
          type: "TXT"
          value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@mycompany.com"
        
        # MX Records
        - name: "@"
          type: "MX"
          value: "1 smtp.google.com"
        - name: "@"
          type: "MX"
          value: "5 alt1.smtp.google.com"
```

### 2. Security Configuration

Implement security-focused DNS configurations:

```yaml
# Cloudflare-only ingress protection
stacks:
  production:
    config:
      domain: api.mycompany.com
      cloudExtras:
        securityGroup:
          ingress:
            allowOnlyCloudflare: true  # Only allow Cloudflare IPs
```

### 3. Multi-Region DNS

Configure DNS for multi-region deployments:

```yaml
# server.yaml - Multi-region setup
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "mycompany.com"
      dnsRecords:
        # US Region
        - name: "us-api"
          type: "CNAME"
          value: "us-east-1-lb.aws.mycompany.com"
        
        # EU Region
        - name: "eu-api"
          type: "CNAME"
          value: "eu-west-1-lb.aws.mycompany.com"
        
        # Geographic routing (requires Cloudflare enterprise)
        - name: "api"
          type: "CNAME"
          value: "us-api.mycompany.com"
          geoPolicy: "US"
        - name: "api"
          type: "CNAME"
          value: "eu-api.mycompany.com"
          geoPolicy: "EU"
```

## DNS Record Types

Simple Container supports all common DNS record types:

### A Records
```yaml
dnsRecords:
  - name: "@"
    type: "A"
    value: "203.0.113.10"
  - name: "www"
    type: "A"
    value: "203.0.113.10"
```

### CNAME Records
```yaml
dnsRecords:
  - name: "api"
    type: "CNAME"
    value: "api-lb.us-east-1.elb.amazonaws.com"
  - name: "www"
    type: "CNAME"
    value: "mycompany.com"
```

### TXT Records
```yaml
dnsRecords:
  - name: "@"
    type: "TXT"
    value: "v=spf1 include:_spf.google.com ~all"
  - name: "_verification"
    type: "TXT"
    value: "google-site-verification=abc123..."
```

### MX Records
```yaml
dnsRecords:
  - name: "@"
    type: "MX"
    value: "1 smtp.google.com"
  - name: "@"
    type: "MX"
    value: "10 alt1.smtp.google.com"
```

## Multi-Tenant DNS

### Customer-Specific Domains

Configure per-customer domains:

```yaml
# client.yaml - Multi-tenant application
stacks:
  customer-a:
    type: cloud-compose
    parent: myorg/saas-infrastructure
    config:
      dockerComposeFile: docker-compose.yaml
      domain: customera.myapp.com
      uses: [mongodb-cluster-us]
      runs: [app]
      secrets:
        CUSTOMER_SETTINGS: "${env:CUSTOMER_A_SETTINGS}"

  customer-b:
    type: cloud-compose
    parent: myorg/saas-infrastructure
    config:
      dockerComposeFile: docker-compose.yaml
      domain: customerb.myapp.com
      uses: [mongodb-cluster-eu]
      runs: [app]
      secrets:
        CUSTOMER_SETTINGS: "${env:CUSTOMER_B_SETTINGS}"
```

### Wildcard Domains

Support subdomain-based multi-tenancy:

```yaml
# server.yaml - Wildcard configuration
resources:
  registrar:
    type: cloudflare
    config:
      credentials: "${auth:cloudflare}"
      accountId: "${secret:cloudflare-account-id}"
      zoneName: "myapp.com"
      dnsRecords:
        - name: "*"
          type: "CNAME"
          value: "tenant-lb.us-east-1.elb.amazonaws.com"
```

## Deployment Integration

### Automatic DNS Updates

DNS records are automatically managed during deployment:

```bash
# Deploy with DNS configuration
sc deploy -s myapp -e production

# DNS records are automatically:
# 1. Created for new domains
# 2. Updated for changed load balancers
# 3. Verified for SSL certificate validation
```

### SSL Certificate Integration

Simple Container automatically handles SSL certificates with DNS validation:

```yaml
# Automatic SSL with DNS validation
stacks:
  production:
    config:
      domain: api.mycompany.com  # SSL cert auto-requested via DNS validation
      sslPolicy: "automatic"     # Default: automatic certificate management
```

## Monitoring and Troubleshooting

### DNS Propagation

Check DNS propagation after deployment:

```bash
# Check DNS propagation
dig api.mycompany.com @8.8.8.8
nslookup api.mycompany.com
```

### Cloudflare Dashboard Integration

Monitor DNS through Cloudflare dashboard:

1. **Analytics**: DNS query analytics and performance metrics
2. **Security**: DDoS protection and security events
3. **Cache**: CDN cache hit ratios and purge controls
4. **SSL/TLS**: Certificate status and security settings

### Common Issues

#### DNS Not Resolving
```bash
# Check if nameservers are correctly set
dig NS mycompany.com

# Verify Cloudflare integration
sc stack get-config -s infrastructure --explain
```

#### SSL Certificate Issues
```bash
# Check certificate status
openssl s_client -connect api.mycompany.com:443 -servername api.mycompany.com

# Verify DNS validation records
dig _acme-challenge.api.mycompany.com TXT
```

## Best Practices

### 1. Environment Naming

Use consistent environment-based domain patterns:

```yaml
# Good - Clear environment prefixes
staging: staging-api.mycompany.com
production: api.mycompany.com

# Avoid - Unclear naming
staging: api-stg.mycompany.com
production: api-prod.mycompany.com  # Should be root domain
```

### 2. Security Configuration

Always enable security features for production:

```yaml
# Production security setup
stacks:
  production:
    config:
      domain: api.mycompany.com
      cloudExtras:
        securityGroup:
          ingress:
            allowOnlyCloudflare: true
        proxy:
          enabled: true      # Enable Cloudflare proxy
          sslMode: "strict"  # Strict SSL/TLS
```

### 3. Template Placeholder Usage

Use placeholders for maintainable configurations:

```yaml
# Good - Dynamic and maintainable
domain: "${stack:name}.mycompany.com"
baseUrl: "https://${stack:name}.mycompany.com"

# Avoid - Hardcoded and error-prone
domain: "staging.mycompany.com"  # Must be changed per environment
```

### 4. Email Infrastructure Planning

Plan email infrastructure early:

```yaml
# Include email setup in initial DNS configuration
dnsRecords:
  # Email delivery
  - name: "@"
    type: "MX"
    value: "1 smtp.google.com"
  
  # Email authentication
  - name: "@"
    type: "TXT"
    value: "v=spf1 include:_spf.google.com ~all"
  
  # Email security
  - name: "_dmarc"
    type: "TXT"
    value: "v=DMARC1; p=quarantine; rua=mailto:dmarc@mycompany.com"
```

## Real-World Examples

### E-commerce Platform
```yaml
# Multi-service e-commerce DNS
dnsRecords:
  - name: "@"
    type: "A"
    value: "203.0.113.10"        # Main site
  - name: "api"
    type: "CNAME"
    value: "api-lb.aws.com"      # API service
  - name: "admin"
    type: "CNAME"
    value: "admin-lb.aws.com"    # Admin panel
  - name: "cdn"
    type: "CNAME"
    value: "d123.cloudfront.net" # CDN assets
```

### SaaS Application
```yaml
# Multi-tenant SaaS DNS
dnsRecords:
  - name: "*"
    type: "CNAME"
    value: "saas-lb.aws.com"     # Tenant subdomains
  - name: "app"
    type: "CNAME"
    value: "app-lb.aws.com"      # Main application
  - name: "docs"
    type: "CNAME"
    value: "docs-s3.aws.com"     # Documentation site
```

## Next Steps

After setting up DNS management:

1. **Configure SSL certificates** for secure HTTPS connections
2. **Set up monitoring** for DNS resolution and performance
3. **Implement CDN caching** strategies for optimal performance
4. **Plan disaster recovery** with backup DNS providers
5. **Review security settings** regularly for Cloudflare protection

## Need Help?

- Review **[Parent Stack Guides](parent-ecs-fargate.md)** for infrastructure setup
- Check **[Secrets Management](secrets-management.md)** for credential handling
- Explore **[Examples](../examples/README.md)** for real-world DNS configurations
- Contact [support@simple-container.com](mailto:support@simple-container.com) for assistance

## Related Resources

- **[Cloudflare DNS API Documentation](https://developers.cloudflare.com/api/operations/dns-records-for-a-zone-list-dns-records)**
- **[Template Placeholders](../concepts/template-placeholders.md)** for dynamic configuration
- **[Multi-Region Deployment Examples](../examples/parent-stacks/aws-multi-region/README.md)**
- **[Security Best Practices](../advanced/best-practices.md)**