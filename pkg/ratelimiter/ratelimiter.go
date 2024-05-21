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
	"sync"
	"time"
)

// RateLimiter will rate limit by delaying queue additions by a specified time
// interval plus the timestamp of a previously added item. For example, if there
// are 2 queue additions with the delayInterval set to 1 second, the first item
// will be added after a 1 second sleep, the second will be added after the
// first item delay along with another 1 second delay interval addition
// resulting in a 2 second sleep.
type RateLimiter struct {
	failuresLock  sync.Mutex
	failures      map[interface{}]int
	currentDelay  time.Time
	delayInterval time.Duration
}

// NewRateLimiter will return a new rate limiter object to be used with the
// workqueue
func NewRateLimiter(delayInterval time.Duration) *RateLimiter {
	return &RateLimiter{
		failures:      map[interface{}]int{},
		currentDelay:  time.Now().Add(-1 * delayInterval),
		delayInterval: delayInterval,
	}
}

// When returns the time when the item should be added onto the workqueue. Uses
// the previously set time to calculate the new sleep interval.
func (r *RateLimiter) When(item interface{}) time.Duration {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	r.failures[item] = r.failures[item] + 1

	now := time.Now()
	newDelay := r.currentDelay.Add(r.delayInterval)
	// directly return 0 if currentDelay + delayInterval is in the past
	if now.After(newDelay) {
		r.currentDelay = now
		return 0
	}

	r.currentDelay = newDelay
	return r.currentDelay.Sub(now)
}

// Forget removes the failure count for an item
func (r *RateLimiter) Forget(item interface{}) {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	delete(r.failures, item)
}

// NumRequeues returns the amount of retries for an item
func (r *RateLimiter) NumRequeues(item interface{}) int {
	r.failuresLock.Lock()
	defer r.failuresLock.Unlock()

	return r.failures[item]
}