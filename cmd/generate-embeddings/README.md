# Simple Container AI Assistant - Embeddings Generator

This tool generates high-quality vector embeddings for Simple Container documentation using OpenAI's embedding models. The generated embeddings power the AI Assistant's semantic search capabilities.

## Features

- **OpenAI Integration**: Uses OpenAI's state-of-the-art embedding models
- **Batch Processing**: Efficiently processes documents in batches with rate limiting
- **Cost Estimation**: Shows estimated costs before generation
- **Flexible Configuration**: Support for multiple embedding models
- **Verbose Output**: Detailed progress reporting and statistics
- **Dry Run Mode**: Preview what would be generated without API calls

## Supported Embedding Models

| Model                    | Dimensions | Cost per 1K tokens | Best for                           |
|--------------------------|------------|--------------------|------------------------------------|
| `text-embedding-3-small` | 1536       | $0.00002           | General use, cost-effective        |
| `text-embedding-3-large` | 3072       | $0.00013           | Higher quality, better performance |
| `text-embedding-ada-002` | 1536       | $0.0001            | Legacy model                       |

## Usage

### Via Welder Build System (Recommended)

```bash
# Set your OpenAI API key in secrets
sc secrets add openai-api-key sk-your-key-here

# Optional: Configure embedding model
export SIMPLE_CONTAINER_EMBEDDING_MODEL=text-embedding-3-small

# Generate embeddings as part of build process
welder run generate-embeddings
```

### Direct Command Usage

```bash
# Build the generator
go build -o bin/generate-embeddings ./cmd/generate-embeddings

# Generate embeddings with default model
./bin/generate-embeddings -openai-key sk-your-key-here -verbose

# Use different model and custom output
./bin/generate-embeddings \
  -openai-key sk-your-key-here \
  -model text-embedding-3-large \
  -output custom/path/embeddings.json \
  -verbose

# Dry run to estimate cost
./bin/generate-embeddings \
  -model text-embedding-3-small \
  -dry-run
```

### Command Line Options

```bash
Usage of generate-embeddings:
  -batch-size int
        Number of documents to process in each batch (default 100)
  -dry-run
        Show what would be done without making API calls
  -model string
        OpenAI embedding model to use (default "text-embedding-3-small")
  -openai-key string
        OpenAI API key (or set OPENAI_API_KEY env var)
  -output string
        Output path for generated embeddings (default "pkg/assistant/embeddings/vectors/prebuilt_embeddings.json")
  -verbose
        Enable verbose output
```

## Output Format

The tool generates embeddings in the format expected by the AI Assistant:

```json
{
  "version": "1.0",
  "documents": [
    {
      "id": "getting-started/installation.md",
      "content": "# Installation\n\nSimple Container can be installed...",
      "metadata": {
        "title": "Installation",
        "path": "getting-started/installation.md",
        "type": "documentation"
      },
      "embedding": [0.123, -0.456, 0.789, ...]
    }
  ]
}
```

## Cost Estimation

The tool provides cost estimates for different models:

- **text-embedding-3-small**: ~$0.02 for ~30 docs (very affordable)
- **text-embedding-3-large**: ~$0.13 for ~30 docs (higher quality)
- **text-embedding-ada-002**: ~$0.10 for ~30 docs (legacy pricing)

Use `--dry-run` to see exact cost estimates for your documentation.

## Integration with AI Assistant

Generated embeddings are automatically used by:

- `sc assistant search` - Semantic documentation search
- `sc assistant chat` - Context-aware assistance 
- `sc assistant dev` - Documentation-enriched file generation
- MCP server - IDE integration with semantic context

## Configuration

### Environment Variables

```bash
# Required
export OPENAI_API_KEY=sk-your-openai-api-key-here

# Optional - embedding model selection  
export SIMPLE_CONTAINER_EMBEDDING_MODEL=text-embedding-3-small
```

### Secrets Management

For production builds, store the API key securely:

```bash
# Add to Simple Container secrets
sc secrets add openai-api-key sk-your-key-here

# The welder task will automatically retrieve it
welder run generate-embeddings
```

## Development

The embeddings generator:

1. **Loads Documentation**: Scans embedded docs from the `docs/` directory
2. **Prepares Batches**: Groups documents to respect API rate limits
3. **Calls OpenAI API**: Uses the embeddings endpoint with specified model
4. **Saves Results**: Writes embeddings in the AI Assistant's expected format
5. **Reports Statistics**: Shows token usage, cost, and performance metrics

## Troubleshooting

### Common Issues

**API Key Not Found**
```bash
❌ Error: OpenAI API key is required
```
Solution: Set `OPENAI_API_KEY` env var or use `-openai-key` flag

**Unsupported Model**
```bash
❌ Error: unsupported embedding model: gpt-4
```
Solution: Use a supported embedding model (see table above)

**Rate Limiting**
The tool automatically handles rate limiting with 500ms delays between batches.

**Large Documents**
Documents longer than 8000 characters are automatically truncated to stay within token limits.

### Verification

After generation, verify embeddings are loaded:

```bash
# Check if AI Assistant loads embeddings
sc assistant search "docker compose"

# Should show semantic search results from your documentation
```

## Performance

- **Speed**: ~100 documents per batch, ~1-2 seconds per batch
- **Memory**: Minimal memory usage, streaming processing
- **Storage**: ~1-5MB output file for typical documentation corpus
- **Cost**: Very affordable - typically under $0.10 for full documentation

## Integration Examples

### CI/CD Pipeline

```yaml
# .github/workflows/build.yml
- name: Generate AI Assistant Embeddings
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    SIMPLE_CONTAINER_EMBEDDING_MODEL: text-embedding-3-small
  run: |
    welder run generate-embeddings
```

### Development Workflow

```bash
# Daily development with fresh embeddings
export OPENAI_API_KEY=sk-your-key
export SIMPLE_CONTAINER_EMBEDDING_MODEL=text-embedding-3-small

# Regenerate after documentation updates  
welder run generate-embeddings

# Test AI Assistant with fresh embeddings
sc assistant chat
```

This tool ensures your AI Assistant always has access to high-quality, semantically searchable documentation embeddings for the best possible user experience.
