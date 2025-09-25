# Documentation Site Example

This example shows how to deploy a static documentation site using MkDocs output.

## Configuration

- **Type**: Static website deployment
- **Bundle Directory**: `${git:root}/docs/site` - MkDocs build output
- **Domain**: `docs.example.com`
- **Location**: European region (EUROPE-CENTRAL2)
- **Error Handling**: Custom 404.html page

## Key Features

- MkDocs documentation deployment
- GCP static hosting
- European region hosting
- Custom error document

## Usage

1. Build your MkDocs documentation: `mkdocs build`
2. Deploy using the parent stack that provides the `dist` template
3. The site will be available at `docs.example.com`

## Parent Stack Requirements

This example requires a parent stack that provides:
- `dist` template with static website deployment capabilities
- GCP static hosting configuration
- Domain management
