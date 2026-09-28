#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
: "${TEACHER_PASSWORD:?请设置 TEACHER_PASSWORD}"
: "${STUDENT_PASSWORD:?请设置 STUDENT_PASSWORD}"

work_dir="$(mktemp -d)"
cleanup() {
  local status=$?
  trap - EXIT
  rm -rf "$work_dir"
  exit "$status"
}
trap cleanup EXIT

teacher_cookie="$work_dir/teacher.cookie"
student_cookie="$work_dir/student.cookie"
student_b_cookie="$work_dir/student-b.cookie"
material_file="$work_dir/verification.md"

cat >"$material_file" <<'EOF'
# CampusClaw 验收材料

这是一份由自动验收脚本上传的 UTF-8 Markdown 材料。
EOF

request_status() {
  curl --silent --output "$1" --write-out "%{http_code}" "${@:2}"
}

assert_status() {
  local actual="$1"
  local expected="$2"
  local label="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "FAIL  ${label}：期望 $expected，实际 $actual" >&2
    exit 1
  fi
  echo "PASS  ${label}（HTTP ${actual}）"
}

health_status="$(request_status "$work_dir/health.json" "$BASE_URL/health")"
assert_status "$health_status" "200" "公开健康检查"

unauthorized_status="$(request_status "$work_dir/unauthorized.json" "$BASE_URL/api/materials")"
assert_status "$unauthorized_status" "401" "未登录访问材料列表"

teacher_login_status="$(request_status "$work_dir/teacher-login.json" \
  --cookie-jar "$teacher_cookie" \
  --header "Content-Type: application/json" \
  --data "{\"username\":\"teacher_a\",\"password\":\"$TEACHER_PASSWORD\"}" \
  "$BASE_URL/api/login")"
assert_status "$teacher_login_status" "200" "教师登录"

student_login_status="$(request_status "$work_dir/student-login.json" \
  --cookie-jar "$student_cookie" \
  --header "Content-Type: application/json" \
  --data "{\"username\":\"student_a1\",\"password\":\"$STUDENT_PASSWORD\"}" \
  "$BASE_URL/api/login")"
assert_status "$student_login_status" "200" "A 班学生登录"

student_b_login_status="$(request_status "$work_dir/student-b-login.json" \
  --cookie-jar "$student_b_cookie" \
  --header "Content-Type: application/json" \
  --data "{\"username\":\"student_b1\",\"password\":\"$STUDENT_PASSWORD\"}" \
  "$BASE_URL/api/login")"
assert_status "$student_b_login_status" "200" "B 班学生登录"

upload_status="$(request_status "$work_dir/upload.json" \
  --cookie "$teacher_cookie" \
  --form "file=@$material_file;type=text/markdown" \
  "$BASE_URL/api/materials")"
assert_status "$upload_status" "201" "教师上传 Markdown"

material_id="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["material"]["id"])' "$work_dir/upload.json")"

student_upload_status="$(request_status "$work_dir/student-upload.json" \
  --cookie "$student_cookie" \
  --form "file=@$material_file;type=text/markdown" \
  "$BASE_URL/api/materials")"
assert_status "$student_upload_status" "403" "学生上传被拒绝"

same_class_detail_status="$(request_status "$work_dir/detail.json" \
  --cookie "$student_cookie" \
  "$BASE_URL/api/materials/$material_id")"
assert_status "$same_class_detail_status" "200" "同班学生查看上传材料"

cross_class_status="$(request_status "$work_dir/cross-class.json" \
  --cookie "$student_b_cookie" \
  "$BASE_URL/api/materials/$material_id")"
assert_status "$cross_class_status" "404" "跨班按 ID 访问被隐藏"

download_status="$(request_status "$work_dir/download.md" \
  --cookie "$student_cookie" \
  "$BASE_URL/api/materials/$material_id/file")"
assert_status "$download_status" "200" "同班学生鉴权下载"

static_path_status="$(request_status "$work_dir/static-path.txt" \
  "$BASE_URL/uploads/seed-a-class.md")"
assert_status "$static_path_status" "200" "静态上传路径由 SPA 接管且不返回原文件"
if grep -q "欢迎进入高一" "$work_dir/static-path.txt"; then
  echo "FAIL  /uploads 路径暴露了原文件" >&2
  exit 1
fi

echo
echo "CampusClaw 关键主路径与失败路径验收通过。"
