# Dynatrace AI & LLM Observability Quick Setup Guide
## Using Traceloop's OpenLLMetry OpenTelemetry SDK for Python

### Overview
This guide demonstrates how to instrument Python applications with OpenLLMetry to observe Large Language Models (LLMs) using Dynatrace's full-stack observability platform.

**OpenLLMetry** bridges the gap between standard OpenTelemetry instrumentation and AI-specific observability by capturing crucial KPIs like model name, version, prompt/completion tokens, and temperature parameters.

---

## Prerequisites

- Python environment
- Dynatrace environment (SaaS or Managed)
- OpenAI API key (if using OpenAI models)
- Access to create Dynatrace tokens

---

## Step 1: Create Dynatrace Access Token

### Required Permissions
Your token needs the following scopes:

1. Navigate to **Access Tokens** in Dynatrace (use `Ctrl/Cmd+K` to search)
2. Select **Generate new token**
3. Enter a **Token name** (e.g., "OpenLLMetry-Python")
4. Add the following scopes:
   - ✅ **Ingest metrics** (`metrics.ingest`)
   - ✅ **Ingest logs** (`logs.ingest`)
   - ✅ **Ingest OpenTelemetry traces** (`openTelemetryTrace.ingest`)
5. Click **Generate token**
6. **Copy the token immediately** (you can only see it once)
7. Store securely in a password manager

---

## Step 2: Install OpenLLMetry SDK

```bash
pip install traceloop-sdk
```

### Supported AI Frameworks
OpenLLMetry automatically instruments:
- OpenAI
- LangChain
- HuggingFace
- Pinecone
- And more...

---

## Step 3: Configure Metrics Temporality

**CRITICAL**: Dynatrace requires delta aggregation temporality for metrics.

Set the environment variable:

```bash
export OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta
```

Or in your Python code:

```python
import os
os.environ['OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE'] = 'delta'
```

---

## Step 4: Initialize OpenLLMetry

Add this code at the **beginning** of your main file:

```python
from traceloop.sdk import Traceloop

# Set metrics temporality preference
import os
os.environ['OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE'] = 'delta'

# Configure authentication headers
headers = {
    "Authorization": "Api-Token <YOUR_DT_API_TOKEN>"
}

# Initialize Traceloop
Traceloop.init(
    app_name="<your-service-name>",
    api_endpoint="https://<YOUR_ENV>.live.dynatrace.com/api/v2/otlp",
    headers=headers
)
```

### Configuration Parameters

| Parameter | Description | Example |
|-----------|-------------|---------|
| `app_name` | Your service/application name | `"my-llm-app"` |
| `api_endpoint` | Dynatrace OTLP endpoint | `"https://abc12345.live.dynatrace.com/api/v2/otlp"` |
| `headers` | Authorization with API token | `{"Authorization": "Api-Token dt0c01..."}` |
| `disable_batch` | Optional: Disable batching for testing | `True` |

---

## Step 5: Instrument Your Code (Optional)

While OpenLLMetry provides auto-instrumentation, you can add custom annotations for enhanced observability:

### Using Decorators

```python
from traceloop.sdk.decorators import workflow, task

@task(name="prepare_prompt")
def prepare_prompt(company_name, max_length):
    """Custom task annotation for detailed tracking"""
    # Your prompt preparation logic
    return prompt

@workflow(name="llm_workflow")
def run_llm_workflow():
    """Workflow annotation for end-to-end tracking"""
    # Your LLM workflow logic
    return result
```

---

## Complete Example: OpenAI with LangChain

```python
import os
import openai
from langchain.chat_models import ChatOpenAI
from langchain.prompts import ChatPromptTemplate
from traceloop.sdk import Traceloop
from traceloop.sdk.decorators import workflow, task

# Configure metrics temporality
os.environ['OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE'] = 'delta'

# Initialize Traceloop
headers = {"Authorization": "Api-Token <YOUR_DT_API_TOKEN>"}
Traceloop.init(
    app_name="langchain-demo",
    api_endpoint="https://<YOUR_ENV>.live.dynatrace.com/api/v2/otlp",
    headers=headers,
    disable_batch=True
)

# Set OpenAI API key
openai.api_key = os.getenv("OPENAI_API_KEY")

@task(name="add_prompt_context")
def add_prompt_context():
    """Create prompt template and model chain"""
    prompt = ChatPromptTemplate.from_template(
        "explain the business of company {company} in a max of {length} words"
    )
    model = ChatOpenAI()
    chain = prompt | model
    return chain

@task(name="prep_prompt_chain")
def prep_prompt_chain():
    """Prepare the prompt chain"""
    return add_prompt_context()

@workflow(name="ask_question")
def prompt_question():
    """Execute the LLM workflow"""
    chain = prep_prompt_chain()
    return chain.invoke({"company": "dynatrace", "length": 50})

if __name__ == "__main__":
    result = prompt_question()
    print(result)
```

### Expected Output

```bash
> python langchain_example.py

Traceloop exporting traces to https://<YOUR_ENV>.live.dynatrace.com/api/v2/otlp, authenticating with custom headers
content='Dynatrace is a software intelligence company that provides monitoring and analytics solutions for modern cloud environments...'
```

---

## Step 6: Verify Data in Dynatrace

### View Traces
1. Navigate to **Distributed traces** in Dynatrace
2. Filter by your `app_name` (e.g., "langchain-demo")
3. View detailed traces showing:
   - LLM model used (e.g., `gpt-4o-mini`)
   - Temperature parameter
   - Token usage (prompt + completion)
   - Request latency
   - Task-level breakdowns

### Captured Attributes
OpenLLMetry automatically captures:
- `llm.model_name` - Model identifier
- `llm.model_version` - Model version
- `llm.temperature` - Temperature parameter
- `llm.completion_tokens` - Tokens in response
- `llm.prompt_tokens` - Tokens in prompt
- `llm.total_tokens` - Total tokens used
- All standard OpenTelemetry span attributes

---

## Architecture Overview

```
┌─────────────────┐
│  Python App     │
│  with OpenAI/   │
│  LangChain      │
└────────┬────────┘
         │
         │ Auto-instrumented
         ▼
┌─────────────────┐
│  OpenLLMetry    │
│  SDK Layer      │
└────────┬────────┘
         │
         │ OpenTelemetry Protocol (OTLP)
         ▼
┌─────────────────┐
│  Dynatrace      │
│  Environment    │
│  (Metrics,      │
│  Traces, Logs)  │
└─────────────────┘
```

---

## Troubleshooting

### Issue: No data appearing in Dynatrace

**Solutions:**
1. Verify token has all three required scopes
2. Check `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=delta` is set
3. Confirm endpoint URL is correct (include `/api/v2/otlp`)
4. Validate token is not expired
5. Check application logs for export errors

### Issue: Traces but no metrics

**Solution:**
- Ensure delta temporality is configured BEFORE `Traceloop.init()`

### Issue: Connection timeout

**Solution:**
- Verify network connectivity to Dynatrace endpoint
- Check firewall rules
- Consider using `disable_batch=True` for initial testing

---

## Environment Variables Reference

| Variable | Purpose | Example |
|----------|---------|---------|
| `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE` | Set metrics aggregation | `delta` |
| `OPENAI_API_KEY` | OpenAI authentication | `sk-...` |
| `DT_API_TOKEN` | Dynatrace token (optional storage) | `dt0c01...` |

---

## Best Practices

1. **Security**
   - Never hardcode API tokens in source code
   - Use environment variables or secret management systems
   - Rotate tokens regularly

2. **Performance**
   - Start with `disable_batch=True` for testing
   - Enable batching for production (`disable_batch=False` or omit)
   - Monitor token costs and usage

3. **Observability**
   - Use `@task` and `@workflow` decorators for custom business logic
   - Add meaningful names to track specific operations
   - Correlate LLM traces with application traces

4. **Development Workflow**
   - Test instrumentation in dev environment first
   - Validate data appears in Dynatrace before production deployment
   - Create service-specific tokens for different environments

---

## Additional Resources

- **Dynatrace Docs**: https://docs.dynatrace.com/docs/observe/dynatrace-for-ai-observability
- **OpenLLMetry GitHub**: https://github.com/traceloop/openllmetry
- **OpenTelemetry Python**: https://opentelemetry.io/docs/languages/python/
- **Traceloop Documentation**: https://traceloop.com/docs/

---

## Supported Python Versions

- Python 3.8+
- Compatible with all major LLM frameworks

---

## License

OpenLLMetry is maintained under the Apache 2.0 license by Traceloop.

---

**Last Updated**: December 2025  
**Version**: Based on Dynatrace latest documentation
