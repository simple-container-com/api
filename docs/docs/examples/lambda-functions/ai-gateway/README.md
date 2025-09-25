# AI Gateway Example

This example shows how to deploy an AI gateway service with AWS Bedrock integration, specific IAM roles for AI model access, and response streaming capabilities.

## Configuration

- **Type**: Lambda single-image deployment
- **Template**: Uses `lambda-eu` template from parent stack
- **Timeout**: 60 seconds
- **AI Integration**: AWS Bedrock with Claude 3 Sonnet and Cohere embeddings
- **Streaming**: Response streaming enabled for real-time AI responses

## Key Features

- **AWS Bedrock Integration**: Direct integration with AWS AI services
- **Specific IAM Roles**: Granular permissions for AI model access
  - `bedrock:InvokeModel` - Basic model invocation
  - `bedrock:InvokeModelWithResponseStream` - Streaming responses
  - `bedrock:CreateModelInvocationJob` - Batch processing
- **Response Streaming**: `RESPONSE_STREAM` mode for real-time AI responses
- **Cross-Region Inference**: Enabled for better availability and performance
- **Multiple AI Models**: Claude 3 Sonnet for text, Cohere for embeddings

## Environments

- **Staging**: `staging-ai-gateway.example.com`
- **Production**: `ai-gateway.example.com`

## AI Model Configuration

- **Default Model**: Claude 3 Sonnet (`anthropic.claude-3-sonnet-20240229-v1:0`)
- **Embedding Model**: Cohere Multilingual v3 (`cohere.embed-multilingual-v3`)
- **Cross-Region**: Enabled for failover and performance optimization

## IAM Permissions Required

The Lambda function requires specific Bedrock permissions:
```yaml
awsRoles:
  - "bedrock:InvokeModel"
  - "bedrock:InvokeModelWithResponseStream"
  - "bedrock:CreateModelInvocationJob"
```

## Usage

1. Ensure your AWS account has Bedrock access enabled
2. Configure the required IAM roles for Bedrock access
3. Set up API key authentication
4. Deploy to staging for AI model testing
5. Promote to production when AI responses are validated

## Parent Stack Requirements

This example requires a parent stack that provides:
- `lambda-eu` template with Bedrock IAM role support
- Lambda deployment capabilities with custom IAM roles
- Domain management
- Secrets management for API keys

## Security Considerations

- API key authentication via AWS Secrets Manager
- Granular IAM permissions for Bedrock access only
- Function URL routing for direct access
- Cross-region inference for redundancy
