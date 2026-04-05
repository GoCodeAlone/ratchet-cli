#!/bin/bash
# Demo: Run the code-gen team with real Ollama + Qwen
# Requires: ollama running with qwen3:8b pulled
echo "=== Agent Mesh Demo ==="
echo "This demo requires Ollama running locally with qwen3:8b model."
echo ""
echo "Setup:"
echo "  ratchet provider setup ollama"
echo "  ratchet provider add ollama local-qwen"
echo ""
echo "Run:"
echo "  ratchet team start code-gen --task 'Build a REST API for task management'"
