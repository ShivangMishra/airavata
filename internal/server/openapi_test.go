/**
*
* Licensed to the Apache Software Foundation (ASF) under one
* or more contributor license agreements. See the NOTICE file
* distributed with this work for additional information
* regarding copyright ownership. The ASF licenses this file
* to you under the Apache License, Version 2.0 (the
* "License"); you may not use this file except in compliance
* with the License. You may obtain a copy of the License at
*
* http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing,
* software distributed under the License is distributed on an
* "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
* KIND, either express or implied. See the License for the
* specific language governing permissions and limitations
* under the License.
 */

package server_test

import (
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/apache/airavata/internal/app"
	"github.com/apache/airavata/internal/server"
)

// recordingRouter collects the patterns a route table registers without serving any
// of them.
type recordingRouter struct{ patterns []string }

func (r *recordingRouter) HandleFunc(pattern string, _ func(http.ResponseWriter, *http.Request)) {
	r.patterns = append(r.patterns, pattern)
}

// Holds the route table against docs/openapi.yaml in both directions: a route the
// server answers but the spec omits is invisible to every client generated from it,
// and a documented route the server does not answer is a promise nothing keeps.
//
// Patterns are compared exactly, path parameter names included, because generated
// clients name their arguments after them.
func TestRoutesMatchOpenAPI(t *testing.T) {
	router := &recordingRouter{}
	server.Routes(router, &app.Services{})

	served := map[string]bool{}
	for _, p := range router.patterns {
		served[p] = true
	}

	documented := openAPIOperations(t, "../../docs/openapi.yaml")

	for _, p := range sortedKeys(served) {
		if !documented[p] {
			t.Errorf("route %q is served but missing from docs/openapi.yaml", p)
		}
	}
	for _, p := range sortedKeys(documented) {
		if !served[p] {
			t.Errorf("operation %q is in docs/openapi.yaml but not served", p)
		}
	}
}

// openAPIOperations returns every operation in the spec as a ServeMux pattern,
// "METHOD /path".
func openAPIOperations(t *testing.T, path string) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(spec.Paths) == 0 {
		t.Fatalf("%s declares no paths", path)
	}

	ops := map[string]bool{}
	for p, item := range spec.Paths {
		for key := range item {
			switch key {
			case "get", "post", "put", "patch", "delete":
				ops[strings.ToUpper(key)+" "+p] = true
			}
		}
	}
	return ops
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
