// SPDX-License-Identifier: Apache-2.0

package main

import "time"

type missionRecoveryControlObserver = func(string, time.Time) MissionProjectBaseline
