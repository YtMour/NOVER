#!/usr/bin/env bash
# NOVER 本地一键验证（scripts/verify.ps1 的 POSIX 等价版本，供 Linux/macOS 与 CI 参考）。
#
# 用法：
#   ./scripts/verify.sh              # 全量（需要 Docker 与已启动的服务栈）
#   ./scripts/verify.sh --skip-e2e   # 跳过冒烟（无需 Docker）
#   SKIP_E2E=1 ./scripts/verify.sh   # 同上
#
# 依次执行：工具检查 → Go vet/test → web/console 构建 → 可选端到端冒烟；
# 任一步骤失败时以退出码 1 结束，并输出中文汇总。
set -u

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
READY_URL="http://127.0.0.1:48080/readyz"
SKIP_E2E="${SKIP_E2E:-0}"

for argument in "$@"; do
  case "$argument" in
    --skip-e2e) SKIP_E2E=1 ;;
    -h|--help)
      printf '用法：%s [--skip-e2e]\n' "$0"
      exit 0
      ;;
    *)
      printf '未知参数：%s（可用 --skip-e2e）\n' "$argument" >&2
      exit 2
      ;;
  esac
done

results_name=()
results_status=()
results_detail=()

add_result() {
  results_name+=("$1")
  results_status+=("$2")
  results_detail+=("${3:-}")
}

run_in() {
  dir=$1
  shift
  ( cd "$dir" && "$@" )
}

run_step() {
  name=$1
  shift
  printf '\n==> %s\n' "$name"
  start=$(date +%s)
  if "$@"; then
    elapsed=$(( $(date +%s) - start ))
    printf '[通过] %s（%ss）\n' "$name" "$elapsed"
    add_result "$name" "通过" ""
  else
    elapsed=$(( $(date +%s) - start ))
    printf '[失败] %s（%ss）\n' "$name" "$elapsed" >&2
    add_result "$name" "失败" ""
  fi
}

printf 'NOVER 本地验证\n'
printf '  仓库：%s\n' "$ROOT"
if [ "$SKIP_E2E" = "1" ]; then
  printf '  冒烟：跳过（--skip-e2e）\n'
else
  printf '  冒烟：运行（需要 Docker 与已启动服务）\n'
fi

# ---------------------------------------------------------------------------
# 1. 工具检查
# ---------------------------------------------------------------------------
printf '\n==> 检查本地工具\n'
have_go=0
have_node=0
have_npm=0
have_docker=0
have_curl=0
command -v go >/dev/null 2>&1 && have_go=1
command -v node >/dev/null 2>&1 && have_node=1
command -v npm >/dev/null 2>&1 && have_npm=1
command -v docker >/dev/null 2>&1 && have_docker=1
command -v curl >/dev/null 2>&1 && have_curl=1

missing_tools=""
report_tool() {
  label=$1
  available=$2
  required=$3
  if [ "$available" = "1" ]; then
    printf '  [OK]     %s\n' "$label"
  elif [ "$required" = "1" ]; then
    printf '  [缺失]   %s\n' "$label"
    missing_tools="$missing_tools $label"
  else
    printf '  [可选]   %s（已跳过，当前 --skip-e2e 不需要）\n' "$label"
  fi
}

if [ "$SKIP_E2E" = "1" ]; then
  docker_required=0
  curl_required=0
else
  docker_required=1
  curl_required=1
fi
report_tool "go" "$have_go" 1
report_tool "node" "$have_node" 1
report_tool "npm" "$have_npm" 1
report_tool "docker" "$have_docker" "$docker_required"
report_tool "curl" "$have_curl" "$curl_required"

if [ -n "$missing_tools" ]; then
  add_result "检查本地工具" "失败" "缺少：$missing_tools"
else
  add_result "检查本地工具" "通过" ""
fi

# ---------------------------------------------------------------------------
# 2. Go 后端：vet + test
# ---------------------------------------------------------------------------
if [ "$have_go" = "1" ]; then
  run_step "Go vet（server）" run_in "$ROOT/server" go vet ./...
  run_step "Go test（server）" run_in "$ROOT/server" go test ./...
else
  printf '\n[跳过] Go 检查：未找到 go\n'
  add_result "Go vet（server）" "跳过" "未找到 go"
  add_result "Go test（server）" "跳过" "未找到 go"
fi

# ---------------------------------------------------------------------------
# 3. 前端：web / console 构建
# ---------------------------------------------------------------------------
ensure_node_modules() {
  app_dir=$1
  if [ ! -d "$app_dir/node_modules" ]; then
    ( cd "$app_dir" && npm ci )
  else
    printf '  已存在 node_modules，跳过 npm ci。\n'
  fi
}

build_frontend() {
  app_dir=$1
  ( cd "$app_dir" && npm run build )
}

if [ "$have_node" = "1" ] && [ "$have_npm" = "1" ]; then
  for app in web console; do
    run_step "前端安装依赖：$app" ensure_node_modules "$ROOT/$app"
    run_step "前端构建：$app" build_frontend "$ROOT/$app"
  done
else
  printf '\n[跳过] 前端构建：未找到 node 或 npm\n'
  add_result "前端构建：web" "跳过" "未找到 node 或 npm"
  add_result "前端构建：console" "跳过" "未找到 node 或 npm"
fi

# ---------------------------------------------------------------------------
# 4. 端到端冒烟（默认运行，--skip-e2e 跳过）
# ---------------------------------------------------------------------------
run_smoke() {
  if [ ! -f "$ROOT/.env" ]; then
    printf '缺少 .env；请先执行 cp .env.example .env，再执行 docker compose up -d --build。\n' >&2
    return 1
  fi
  if [ "$have_curl" != "1" ]; then
    printf '未找到 curl，无法探测 API 就绪状态。\n' >&2
    return 1
  fi
  if ! curl -fsS --max-time 3 "$READY_URL" >/dev/null 2>&1; then
    printf 'API 未就绪；请先执行 docker compose up -d --build，并确认 %s 可访问。\n' "$READY_URL" >&2
    return 1
  fi
  if ! command -v pwsh >/dev/null 2>&1; then
    printf '未找到 pwsh；冒烟脚本需要 PowerShell 7（或 Windows PowerShell 5.1）。\n' >&2
    return 1
  fi
  ( cd "$ROOT" && pwsh -NoProfile -File scripts/qa/run-all.ps1 )
}

if [ "$SKIP_E2E" = "1" ]; then
  printf '\n[跳过] 冒烟测试（端到端）：已指定 --skip-e2e\n'
  add_result "冒烟测试（端到端）" "跳过" "已指定 --skip-e2e"
else
  run_step "冒烟测试（端到端）" run_smoke
fi

# ---------------------------------------------------------------------------
# 5. 汇总
# ---------------------------------------------------------------------------
printf '\n================ 验证汇总 ================\n'
failed=0
skipped=0
passed=0
for index in "${!results_name[@]}"; do
  status=${results_status[$index]}
  case "$status" in
    通过) passed=$((passed + 1)) ;;
    失败) failed=$((failed + 1)) ;;
    *) skipped=$((skipped + 1)) ;;
  esac
  printf '  [%s] %s %s\n' "$status" "${results_name[$index]}" "${results_detail[$index]}"
done

printf '\n'
if [ "$failed" -gt 0 ]; then
  printf '结果：%s 项失败，%s 项通过，%s 项跳过。\n' "$failed" "$passed" "$skipped" >&2
  exit 1
fi
printf '结果：全部通过（%s 项通过，%s 项跳过）。\n' "$passed" "$skipped"
exit 0
