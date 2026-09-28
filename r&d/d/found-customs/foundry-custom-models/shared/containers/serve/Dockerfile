# syntax=docker/dockerfile:1
# Serving image for every OpenAI-compatible example: upstream vLLM plus a
# ten-line launcher that finds the mounted model and any LoRA adapters.
# Azure ML builds this in the workspace registry (az ml environment create).
ARG VLLM_IMAGE=vllm/vllm-openai:v0.30.0
FROM ${VLLM_IMAGE}

COPY serve.sh /opt/ftm/serve.sh
RUN chmod 0755 /opt/ftm/serve.sh

# Weights come from the Azure ML model mount, never from the Hub at runtime.
ENV MODEL_ROOT=/models \
    VLLM_PORT=8000 \
    HF_HUB_OFFLINE=1 \
    VLLM_NO_USAGE_STATS=1 \
    DO_NOT_TRACK=1

EXPOSE 8000
ENTRYPOINT ["/opt/ftm/serve.sh"]
