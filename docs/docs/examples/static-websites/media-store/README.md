# Media Store Example

This example shows how to deploy a media-specific static hosting site with custom error handling.

## Configuration

- **Type**: Static website deployment
- **Template**: Uses `static-site` template from parent stack
- **Bundle Directory**: `${git:root}/bundle` - Media assets directory
- **Domain**: `media.example.com` (media-specific subdomain)
- **Custom Error Document**: `error.html` instead of `index.html`
- **Production Only**: Single environment deployment

## Key Features

- Media-specific static hosting
- Custom error document for better UX
- Production-only deployment
- Media-optimized domain naming

## Usage

1. Prepare your media assets in the `bundle/` directory
2. Deploy directly to production
3. The media store will be available at `media.example.com`

## Parent Stack Requirements

This example requires a parent stack that provides:
- `static-site` template
- Static website deployment capabilities
- Domain management for media subdomain
