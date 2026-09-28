# syntax=docker/dockerfile:1
# Training and fetch image. Built on the same vLLM image that serves, so the
# tokenizer, chat template and transformers version used to train are the
# ones used to serve.
ARG VLLM_IMAGE=vllm/vllm-openai:v0.30.0
FROM ${VLLM_IMAGE}

COPY requirements.txt /tmp/ftm-requirements.txt
RUN (python3 -m pip install --no-cache-dir -r /tmp/ftm-requirements.txt \
     || uv pip install --system --no-cache -r /tmp/ftm-requirements.txt) \
 && rm /tmp/ftm-requirements.txt

ENV HF_XET_HIGH_PERFORMANCE=1 \
    VLLM_NO_USAGE_STATS=1 \
    TOKENIZERS_PARALLELISM=false

# Azure ML runs its own command; the vLLM entrypoint must not intercept it.
ENTRYPOINT []
CMD ["/bin/bash"]
