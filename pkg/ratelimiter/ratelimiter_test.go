/*
Copyright 2019, Oath Inc.

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
package ratelimiter

import (
	"testing"
	"time"
)

func TestNewRateLimiter(t *testing.T) {
	rateLimiter := NewRateLimiter(250 * time.Millisecond)
	if rateLimiter.failures == nil {
		t.Error("Rate limiter failures map should not be nil")
	}
	if rateLimiter.delayInterval != 250*time.Millisecond {
		t.Error("Rate limiter delay time should be equal to 250 ms")
	}
	initialCurrentDelay := time.Now().Sub(rateLimiter.currentDelay)
	if initialCurrentDelay > 251*time.Millisecond || initialCurrentDelay < 249*time.Millisecond {
		t.Error("Initial current delay should be 250 ms in the past")
	}
}

func validateMapKey(t *testing.T, key string, rateLimiter *RateLimiter) {
	requeues, exists := rateLimiter.failures[key]
	if exists == false {
		t.Error("Key should exist in map")
	}
	if requeues != 1 {
		t.Error("Num of requeues should be equal to 1")
	}
}

func TestWhen(t *testing.T) {
	rateLimiter := NewRateLimiter(250 * time.Millisecond)
	sleepTime := rateLimiter.When("key-one")
	validateMapKey(t, "key-one", rateLimiter)
	if sleepTime != 0 {
		t.Error("When should return 0 if current delay is in the past")
	}

	sleepTime = rateLimiter.When("key-two")
	validateMapKey(t, "key-two", rateLimiter)
	if sleepTime > 251*time.Millisecond || sleepTime < 249*time.Millisecond {
		t.Error("Sleep time should be around 250 ms")
	}

	sleepTime = rateLimiter.When("key-three")
	validateMapKey(t, "key-three", rateLimiter)
	if sleepTime > 501*time.Millisecond || sleepTime < 499*time.Millisecond {
		t.Error("Sleep time should be around 500 ms")
	}

	time.Sleep(750 * time.Millisecond)
	sleepTime = rateLimiter.When("key-four")
	validateMapKey(t, "key-four", rateLimiter)
	if sleepTime != 0 {
		t.Error("When should return 0 if current delay is in the past")
	}
}

func TestForget(t *testing.T) {
	rateLimiter := NewRateLimiter(250 * time.Millisecond)
	rateLimiter.failures["key"] = 1
	rateLimiter.Forget("key")

	_, exists := rateLimiter.failures["key"]
	if exists == true {
		t.Error("Key should have been removed from the map")
	}
}

func TestNumRequeues(t *testing.T) {
	rateLimiter := NewRateLimiter(250 * time.Millisecond)
	rateLimiter.failures["key"] = 3
	requeues := rateLimiter.NumRequeues("key")
	if requeues != 3 {
		t.Error("Num of requeues should be equal to 3")
	}

	requeues = rateLimiter.NumRequeues("nonexistent-key")
	if requeues != 0 {
		t.Error("Num of requeues should be 0 for nonexistent key")
	}
}