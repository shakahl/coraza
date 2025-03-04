// Copyright 2022 Juan Pablo Tosso
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package collection

import (
	"regexp"
	"sort"
	"testing"

	"github.com/corazawaf/coraza/v3/types/variables"
)

func TestCollectionProxy(t *testing.T) {
	c1 := NewMap(variables.ArgsPost)
	c2 := NewMap(variables.ArgsGet)
	proxy := NewProxy(variables.Args, c1, c2)

	c1.Set("key1", []string{"value1"})
	c1.Set("key2", []string{"value2"})
	c2.Set("key3", []string{"value3"})

	p := proxy.FindAll()
	if len(p) != 3 {
		t.Error("Error finding all")
	} else {
		p := proxy.FindAll()
		m := false
		for _, v := range p {
			if v.Value() == "value1" {
				m = true
				break
			}
		}
		if !m {
			t.Error("Error finding all")
		}
	}
	var f []string
	for _, r := range p {
		f = append(f, r.Value())
	}
	sort.Strings(f)
	if f[0] != "value1" || f[1] != "value2" || f[2] != "value3" {
		t.Error("Error finding all")
	}

	if len(proxy.FindString("key3")) == 0 {
		t.Error("Error finding string")
	}
	if len(proxy.FindRegex(regexp.MustCompile("k.*"))) != 3 {
		t.Error("Error finding regex")
	}
}
