#!/usr/bin/env bash
set -e

# Detect container runtime (Podman or Docker)
if command -v podman &> /dev/null; then
    CR="podman"
elif command -v docker &> /dev/null; then
    CR="docker"
else
    echo "❌ Erro: Nem podman nem docker foram encontrados no sistema."
    exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=========================================================================================================="
echo "🐳 Construindo contêiner de Benchmark com IBM clidriver via ${CR}..."
echo "=========================================================================================================="

${CR} build -t db2-benchmark:latest -f "${SCRIPT_DIR}/Dockerfile.benchmark" "${ROOT_DIR}"

echo ""
echo "=========================================================================================================="
echo "🚀 Executando Suíte Comparativa (go-db2 vs go_ibm_db) contra o Db2 local (127.0.0.1:50000)..."
echo "=========================================================================================================="

${CR} run --rm \
    --network host \
    -e DB2_HOST="127.0.0.1" \
    -e DB2_PORT="50000" \
    -e DB2_DATABASE="TESTDB" \
    -e DB2_USER="db2inst1" \
    -e DB2_PASSWORD="MinhaSenhaForte123" \
    -v "${SCRIPT_DIR}:/workspace/go-db2/benchmarks:Z" \
    db2-benchmark:latest
