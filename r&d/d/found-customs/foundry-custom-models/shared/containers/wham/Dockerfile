# syntax=docker/dockerfile:1
# Microsoft Muse (WHAM): the research server from the microsoft/wham repo,
# fronted by a stdlib shim that adds /health and binds 0.0.0.0.
# The CUDA/PyTorch stack is borrowed from the pinned vLLM image.
ARG VLLM_IMAGE=vllm/vllm-openai:v0.30.0
FROM ${VLLM_IMAGE}

# Python dependencies are those the model repo declares, at a pinned commit.
ARG WHAM_REVISION=main
RUN python3 -c "from huggingface_hub import hf_hub_download as d; print(d('microsoft/wham','requirements.txt',revision='${WHAM_REVISION}',local_dir='/opt/wham'))" \
 && (python3 -m pip install --no-cache-dir -r /opt/wham/requirements.txt \
     || uv pip install --system --no-cache -r /opt/wham/requirements.txt)

COPY entry.sh shim.py /opt/ftm/
RUN chmod 0755 /opt/ftm/entry.sh

ENV MODEL_ROOT=/models HF_HUB_OFFLINE=1 WHAM_CHECKPOINT=models/WHAM_1.6B_v1.ckpt
EXPOSE 8080
ENTRYPOINT ["/opt/ftm/entry.sh"]
