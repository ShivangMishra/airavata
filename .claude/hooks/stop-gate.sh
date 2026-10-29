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
# Stop: when Go code, go.mod/go.sum or migrations have uncommitted changes, require the
# definition of done (gofmt clean, go vet, go test) before Claude finishes. Runs once
# per stop: if Claude is already continuing because of this hook, let it stop.
set -u

input=$(cat)
[[ "$(jq -r '.stop_hook_active // false' <<<"$input")" == "true" ]] && exit 0
cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

changed=$({ git diff --name-only HEAD; git ls-files --others --exclude-standard; } 2>/dev/null |
  grep -E '\.go$|^go\.(mod|sum)$|^internal/db/migrations/' | grep -v '^\.claude/')
[[ -z "$changed" ]] && exit 0

report=""

unformatted=$(gofmt -l . 2>&1)
[[ -n "$unformatted" ]] && report+=$'gofmt -l . lists:\n'"$unformatted"$'\n'

if ! out=$(go vet ./... 2>&1); then
  report+=$'go vet ./... failed:\n'"$(head -40 <<<"$out")"$'\n'
fi

if ! out=$(go test ./... 2>&1); then
  report+=$'go test ./... failed:\n'"$(grep -E -- '^(--- FAIL|FAIL|panic:|\s+\S+_test\.go:[0-9]+:)' <<<"$out" | head -60)"$'\n'
fi

if [[ -n "$report" ]]; then
  printf 'Not done yet: the definition of done in CLAUDE.md is failing.\n%s' "$report" >&2
  exit 2
fi
exit 0
