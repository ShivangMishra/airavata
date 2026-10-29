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
# PreToolUse(Edit|Write): applied migrations are immutable. Creating a new
# internal/db/migrations/NNNN_*.sql is allowed; changing an existing one is blocked.
set -u

input=$(cat)
file=$(jq -r '.tool_input.file_path // empty' <<<"$input")
tool=$(jq -r '.tool_name // empty' <<<"$input")

case "$file" in
  */internal/db/migrations/*.sql | internal/db/migrations/*.sql) ;;
  *) exit 0 ;;
esac

if [[ "$tool" == "Write" && ! -e "$file" ]]; then
  exit 0
fi

cat >&2 <<EOF
$file is an existing migration and may already be applied to a database. Migrations are
never edited: add internal/db/migrations/<next number>_<description>.sql instead, which
corrects or extends it. See the /schema-change skill.
EOF
exit 2
