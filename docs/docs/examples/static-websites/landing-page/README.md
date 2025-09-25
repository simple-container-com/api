# Landing Page Example

This example shows how to deploy a main website with SPA (Single Page Application) configuration.

## Configuration

- **Type**: Static website deployment
- **Bundle Directory**: `${git:root}/public` - Static files directory
- **Domain**: `example.com` (main domain)
- **Location**: European region (EUROPE-CENTRAL2)
- **SPA Configuration**: Same file for index and error documents

## Key Features

- Main website deployment
- SPA configuration (index.html serves all routes)
- European region hosting
- Root domain deployment

## Usage

1. Build your static website files in the `public/` directory
2. Deploy using the parent stack that provides static hosting
3. The site will be available at `example.com`

## Parent Stack Requirements

This example requires a parent stack that provides:
- Static website deployment template
- GCP static hosting configuration
- Domain management for root domain
