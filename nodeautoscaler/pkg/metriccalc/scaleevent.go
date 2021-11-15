/**
 * Copyright (c) 2020 CoCreate LLC
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy of
 * this software and associated documentation files (the "Software"), to deal in
 * the Software without restriction, including without limitation the rights to
 * use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
 * the Software, and to permit persons to whom the Software is furnished to do so,
 * subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
 * FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
 * COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
 * IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
 * CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package metriccalc

import (
	"time"
)

// ScaleT is the type of scale
type ScaleT string

const (
	// scale up
	scaleUp ScaleT = "up"
	// scale down
	scaleDown ScaleT = "down"
	// do nothing
	noScale ScaleT = "stay"
)

type scaleEvent struct {
	// first time to observe a potential scaling
	// usually when calculator got a result that exceeds any threshold
	// if this event is still vaild after a alarm window, it's fired
	firstObservedTime time.Time

	// the predicted time this event should be fired
	shouldFiredTime time.Time

	// the predicted time this event should be canceled
	// if related metrics go back to normal for a certain while
	// this time is renewed each time the threshold is still broken
	shouldCanceledTime time.Time

	eventType ScaleT

	firedTime time.Time

	// next fired scaling must after this time
	allowNextFireTime time.Time
}
