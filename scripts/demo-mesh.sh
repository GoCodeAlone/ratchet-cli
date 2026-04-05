#!/bin/bash
# Demo: Run the code-gen team with real Ollama + Qwen
# Requires: ollama running with qwen3:8b and qwen3:14b pulled
echo "=== Agent Mesh Demo ==="
echo "This demo requires Ollama running locally with qwen3:8b and qwen3:14b models."
echo ""
echo "Setup:"
echo "  ollama pull qwen3:8b"
echo "  ollama pull qwen3:14b"
echo "  ratchet provider setup ollama"
echo "  ratchet provider add ollama local-qwen"
echo ""
echo "Run:"
echo "  ratchet team start code-gen --task 'Build a REST API for task management'"
