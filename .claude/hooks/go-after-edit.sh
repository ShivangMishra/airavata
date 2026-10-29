#!/usr/bin/env bash
#
# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements. See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership. The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License. You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
#
# PostToolUse(Edit|Write): gofmt the edited Go file, vet its package, and require the
# ASF license header on files created with Write. Exit 2 sends the problems to Claude.
set -u

input=$(cat)
file=$(jq -r '.tool_input.file_path // empty' <<<"$input")
tool=$(jq -r '.tool_name // empty' <<<"$input")
[[ "$file" == *.go && -f "$file" ]] || exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

problems=""

if ! out=$(gofmt -l -w "$file" 2>&1); then
  problems+=$'gofmt could not parse the file:\n'"$out"$'\n'
fi

if [[ "$tool" == "Write" ]] && ! grep -q "Licensed to the Apache Software Foundation" "$file"; then
  problems+="$file has no ASF license header. Copy the block comment at the top of cmd/airavata-server/main.go."$'\n'
fi

if [[ -z "$problems" ]] && ! out=$(go vet "$(dirname "$file")" 2>&1); then
  problems+=$'go vet failed for this package (fix it, or carry on if a later edit in this change resolves it):\n'
  problems+=$(head -40 <<<"$out")$'\n'
fi

if [[ -n "$problems" ]]; then
  printf '%s' "$problems" >&2
  exit 2
fi
exit 0
