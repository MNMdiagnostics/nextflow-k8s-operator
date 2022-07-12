/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"math/rand"
)

// Generate a hexadecimal hash of the specified length
func generateHash(n int) string {
	const pool = "0123456789abcdef"
	s := make([]byte, n)
	for i := range s {
		s[i] = pool[rand.Intn(len(pool))]
	}
	return string(s)
}

// Check if a map item is empty
func keyIsEmpty(x map[string]string, key string) bool {
	value, ok := x[key]
	if !ok || value == "" {
		return true
	}
	return false
}
